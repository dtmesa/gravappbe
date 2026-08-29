using Amazon.SimpleEmailV2;
using Amazon.SimpleEmailV2.Model;

namespace Gravity.Api.Common;

public class SesEmailSender : IEmailSender
{
	private readonly IAmazonSimpleEmailServiceV2 _ses;
	private readonly string _from;

	public SesEmailSender(IAmazonSimpleEmailServiceV2 ses, string fromAddress)
	{
		_ses = ses;
		_from = fromAddress;
	}

	public Task SendAsync(string toAddress, string subject, string body, CancellationToken ct = default) =>
		_ses.SendEmailAsync(new SendEmailRequest
		{
			FromEmailAddress = _from,
			Destination = new Destination { ToAddresses = [toAddress] },
			Content = new EmailContent
			{
				Simple = new Message
				{
					Subject = new Content { Data = subject },
					Body = new Body { Text = new Content { Data = body } },
				},
			},
		}, ct);
}
