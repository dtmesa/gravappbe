namespace Gravity.Api.Common;

public interface IEmailSender
{
	Task SendAsync(string toAddress, string subject, string body, CancellationToken ct = default);
}
