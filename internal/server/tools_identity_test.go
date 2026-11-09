package server

import (
	"testing"

	"sarak.sh/mikluko/jmap/mail/identity"
)

func TestFormatIdentity(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ident *identity.Identity
		want  string
	}{
		{
			"signatures",
			&identity.Identity{ID: "b", Name: "Mayor of Saraksh", Email: "saraksh@cities.sarak.sh", TextSignature: "— Mayor\nSaraksh", HTMLSignature: "<p>— Mayor</p>"},
			"Mayor of Saraksh <saraksh@cities.sarak.sh> [id: b]\n  textSignature: \"— Mayor\\nSaraksh\"\n  htmlSignature: \"<p>— Mayor</p>\"\n",
		},
		{
			"no signatures, no name",
			&identity.Identity{ID: "c", Email: "erg@sarak.sh"},
			"(unnamed) <erg@sarak.sh> [id: c]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatIdentity(tc.ident); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
