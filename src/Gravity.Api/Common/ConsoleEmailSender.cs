namespace Gravity.Api.Common;

/// <summary>
/// Local-dev default: logs the email instead of sending it, so the whole
/// recovery/confirmation flow (including reading the OTP) can be exercised via
/// `dotnet run` with no AWS credentials at all. Swapped for SesEmailSender by
/// setting EMAIL_SENDER=ses. Logs the plain-text body -- the HTML version
/// renders correctly in a real inbox but is noise in a terminal.
/// </summary>
public class ConsoleEmailSender : IEmailSender
{
	private readonly ILogger<ConsoleEmailSender> _logger;

	public ConsoleEmailSender(ILogger<ConsoleEmailSender> logger) => _logger = logger;

	public Task SendAsync(string toAddress, string subject, EmailBody body, CancellationToken ct = default)
	{
		_logger.LogInformation("[Email] To: {To} | Subject: {Subject}\n{Body}", toAddress, subject, body.Text);
		return Task.CompletedTask;
	}
}
