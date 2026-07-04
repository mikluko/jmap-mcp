package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mikluko/jmap"
)

// attachmentURLTTL bounds the life of a signed attachment URL. Deliberately
// short: the URL is an unauthenticated capability carrying a sealed JMAP
// token, meant to be fetched immediately after issuance, not shared or stored.
const attachmentURLTTL = 30 * time.Second

// attachmentURLer seals attachment download claims into opaque expiring URL
// tokens and opens them back. Claims are AES-GCM encrypted: they carry the
// JMAP bearer token, which must never appear in a URL in the clear.
type attachmentURLer struct {
	aead cipher.AEAD
}

// attachmentClaims is the sealed payload of a signed attachment URL. Short
// JSON keys keep the resulting URL compact.
type attachmentClaims struct {
	Token   string `json:"t"` // JMAP bearer token
	Account string `json:"a"`
	Blob    string `json:"b"`
	Name    string `json:"n"`
	Type    string `json:"c"`
	Exp     int64  `json:"e"` // unix seconds
}

// newAttachmentURLer derives an AES-256 key from secret, or generates a
// random per-process key when secret is empty (URLs then do not survive
// restarts, acceptable given the short TTL; set a secret for multi-replica
// deployments).
func newAttachmentURLer(secret string) (*attachmentURLer, error) {
	var key [32]byte
	if secret != "" {
		key = sha256.Sum256([]byte(secret))
	} else if _, err := rand.Read(key[:]); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &attachmentURLer{aead: aead}, nil
}

// seal encrypts claims into a URL-safe opaque token.
func (u *attachmentURLer) seal(c *attachmentClaims) (string, error) {
	plain, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, u.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := u.aead.Seal(nonce, nonce, plain, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// open decrypts an opaque token. Tampered or undecodable tokens fail;
// expiry is checked separately by the caller via claims.Exp.
func (u *attachmentURLer) open(token string) (*attachmentClaims, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(sealed) < u.aead.NonceSize() {
		return nil, fmt.Errorf("malformed token")
	}
	nonce, ct := sealed[:u.aead.NonceSize()], sealed[u.aead.NonceSize():]
	plain, err := u.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}
	c := &attachmentClaims{}
	if err := json.Unmarshal(plain, c); err != nil {
		return nil, fmt.Errorf("invalid token")
	}
	return c, nil
}

// AttachmentHandler serves GET /attachments/{token} by streaming the blob
// from the JMAP server to the client. The sealed token is the sole access
// control; no other authentication is applied.
func (s *Server) AttachmentHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.attachmentURL == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		claims, err := s.attachmentURL.open(strings.TrimPrefix(r.URL.Path, "/attachments/"))
		if err != nil {
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}
		if time.Now().Unix() > claims.Exp {
			http.Error(w, "link expired", http.StatusGone)
			return
		}

		client := (&jmap.Client{SessionEndpoint: s.sessionURL}).WithAccessToken(claims.Token)
		body, err := client.DownloadWithContext(r.Context(), jmap.ID(claims.Account), jmap.ID(claims.Blob))
		if err != nil {
			http.Error(w, "upstream download failed", http.StatusBadGateway)
			return
		}
		defer body.Close()

		contentType := claims.Type
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sanitizeFilename(claims.Name)))
		io.Copy(w, body)
	})
}

func sanitizeFilename(name string) string {
	if name == "" {
		return "attachment"
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == '"' || r == '\\' || r == '/' {
			return '_'
		}
		return r
	}, name)
}
