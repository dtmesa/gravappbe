package mail

import (
	"bytes"
	"html/template"
)

// Self-contained HTML with inline styles and a table layout, for broad
// mail-client compatibility. Colours match the app's palette. Fonts are
// web-safe rather than the app's Google Fonts: mail clients largely ignore
// @font-face, so anything else would silently fall back anyway.
//
// html/template escapes interpolated values. The C# version interpolated
// unescaped, so a username containing markup was injected into the
// forgot-username email; that no longer happens.
var layout = template.Must(template.New("email").Parse(`<!DOCTYPE html>
<html>
<body style="margin:0;padding:0;background-color:#000000;font-family:'Trebuchet MS',Arial,sans-serif;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#000000;padding:40px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%" style="max-width:420px;" cellpadding="0" cellspacing="0">
          <tr>
            <td align="center" style="padding-bottom:28px;">
              <span style="font-family:'Trebuchet MS',Arial,sans-serif;font-weight:700;font-size:36px;letter-spacing:3px;color:#6d28d9;">GRAVITY</span>
            </td>
          </tr>
          <tr>
            <td>
              <p style="color:#9A98AD;font-family:'Trebuchet MS',Arial,sans-serif;font-weight:400;font-size:16px;line-height:24px;text-align:center;margin:0 0 16px;">{{.Intro}}</p>
              <div style="text-align:center;margin:8px 0 24px;">
                <span style="display:inline-block;background-color:#191228;color:#CFCFCF;font-size:28px;font-weight:700;letter-spacing:6px;padding:14px 22px;border-radius:12px;font-family:Consolas,'Courier New',monospace;">{{.Code}}</span>
              </div>
              <p style="color:#9A98AD;font-family:'Trebuchet MS',Arial,sans-serif;font-weight:400;font-size:14px;line-height:22px;text-align:center;margin:24px 0 0;">{{.Footer}}</p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`))

const (
	ignoreNote = "If you didn't request this, you can safely ignore this email."
	expiryNote = "This code expires in 15 minutes."
)

func render(intro, code, footer string) string {
	var buf bytes.Buffer

	// The template is a constant and the data are plain strings, so execution
	// cannot fail.
	_ = layout.Execute(&buf, struct{ Intro, Code, Footer string }{intro, code, footer})

	return buf.String()
}

func ForgotUsername(username string) Body {
	const intro = "You (or someone using this email) requested your Gravity username."

	return Body{
		HTML: render(intro, username, ignoreNote),
		Text: intro + "\n\nYour username is: " + username + "\n\n" + ignoreNote,
	}
}

func PasswordReset(code string) Body {
	const intro = "Use this code to reset your Gravity password."

	return Body{
		HTML: render(intro, code, expiryNote+" "+ignoreNote),
		Text: intro + "\n\nYour password reset code is: " + code + "\n\n" + expiryNote + " " + ignoreNote,
	}
}

func EmailConfirmation(code string) Body {
	const intro = "Enter this code in the app to confirm this email address."

	return Body{
		HTML: render(intro, code, expiryNote+" "+ignoreNote),
		Text: intro + "\n\nYour confirmation code is: " + code + "\n\n" + expiryNote + " " + ignoreNote,
	}
}
