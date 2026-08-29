namespace Gravity.Api.Common;

/// <summary>
/// Every email is sent as both HTML and plain text (better deliverability and
/// accessibility than HTML-only) -- this pairs the two so callers can't send
/// one without the other.
/// </summary>
public record EmailBody(string Html, string Text);

/// <summary>
/// Simple, self-contained HTML for every email the app sends -- inline styles
/// and a table-based layout for broad client compatibility. Colors match the
/// app's own palette (workout-app/src/css/color.ts). Fonts are web-safe
/// (Trebuchet MS / Consolas) rather than the app's actual Google Fonts (Play,
/// Syncopate): mail clients don't load custom web fonts the way browsers do
/// -- only Apple Mail supports @font-face in mail at all, and even there it's
/// inconsistent -- so anything outside the OS-preinstalled set silently falls
/// back anyway. Monospace for the code/username display isn't just a
/// fallback choice, it's the semantically right font for that content.
/// </summary>
public static class EmailTemplates
{
	private static string Wrap(string bodyHtml) => $$"""
		<!DOCTYPE html>
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
		              {{bodyHtml}}
		            </td>
		          </tr>
		        </table>
		      </td>
		    </tr>
		  </table>
		</body>
		</html>
		""";

	private static string Paragraph(string text) =>
		$"""<p style="color:#9A98AD;font-family:'Trebuchet MS',Arial,sans-serif;font-weight:400;font-size:16px;line-height:24px;text-align:center;margin:0 0 16px;">{text}</p>""";

	private static string CodeBlock(string code) => $"""
		<div style="text-align:center;margin:8px 0 24px;">
		  <span style="display:inline-block;background-color:#191228;color:#CFCFCF;font-size:28px;font-weight:700;letter-spacing:6px;padding:14px 22px;border-radius:12px;font-family:Consolas,'Courier New',monospace;">{code}</span>
		</div>
		""";

	private static string Footer(string note) =>
		$"""<p style="color:#9A98AD;font-family:'Trebuchet MS',Arial,sans-serif;font-weight:400;font-size:14px;line-height:22px;text-align:center;margin:24px 0 0;">{note}</p>""";

	private const string IgnoreNote = "If you didn't request this, you can safely ignore this email.";
	private const string ExpiryNote = "This code expires in 15 minutes.";

	public static EmailBody ForgotUsername(string username) => new(
		Wrap(
			Paragraph("You (or someone using this email) requested your Gravity username.") +
			CodeBlock(username) +
			Footer(IgnoreNote)),
		$"You (or someone using this email) requested your Gravity username.\n\n" +
		$"Your username is: {username}\n\n{IgnoreNote}");

	public static EmailBody PasswordReset(string code) => new(
		Wrap(
			Paragraph("Use this code to reset your Gravity password.") +
			CodeBlock(code) +
			Footer($"{ExpiryNote} {IgnoreNote}")),
		$"Use this code to reset your Gravity password.\n\n" +
		$"Your password reset code is: {code}\n\n{ExpiryNote} {IgnoreNote}");

	public static EmailBody EmailConfirmation(string code) => new(
		Wrap(
			Paragraph("Enter this code in the app to confirm this email address.") +
			CodeBlock(code) +
			Footer($"{ExpiryNote} {IgnoreNote}")),
		$"Enter this code in the app to confirm this email address.\n\n" +
		$"Your confirmation code is: {code}\n\n{ExpiryNote} {IgnoreNote}");
}
