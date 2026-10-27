package mail

import (
	"strings"
	"testing"
)

func TestTemplatesCarryTheCode(t *testing.T) {
	for name, body := range map[string]Body{
		"reset":   PasswordReset("012345"),
		"confirm": EmailConfirmation("012345"),
	} {
		if !strings.Contains(body.HTML, ">012345<") || !strings.Contains(body.Text, "012345") {
			t.Errorf("%s: code missing", name)
		}

		if !strings.Contains(body.Text, "15 minutes") {
			t.Errorf("%s: expiry note missing", name)
		}
	}
}

func TestUsernameIsEscapedInHTML(t *testing.T) {
	body := ForgotUsername(`<b>x</b>`)

	if strings.Contains(body.HTML, "<b>x</b>") {
		t.Error("username markup injected into HTML")
	}

	if !strings.Contains(body.HTML, "&lt;b&gt;x&lt;/b&gt;") {
		t.Error("escaped username missing")
	}

	// Plain text is not HTML, so it stays verbatim.
	if !strings.Contains(body.Text, "<b>x</b>") {
		t.Error("text part altered")
	}
}
