namespace Gravity.Api.Common;

public interface IEmailSender
{
	Task SendAsync(string toAddress, string subject, EmailBody body, CancellationToken ct = default);
}
