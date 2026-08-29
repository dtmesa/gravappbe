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

	public Task SendAsync(string toAddress, string subject, EmailBody body, CancellationToken ct = default) =>
		_ses.SendEmailAsync(new SendEmailRequest
		{
			FromEmailAddress = _from,
			Destination = new Destination { ToAddresses = [toAddress] },
			Content = new EmailContent
			{
				Simple = new Message
				{
					Subject = new Content { Data = subject },
					// Both parts are sent (multipart/alternative) -- HTML for
					// clients that render it, plain text as the fallback for
					// clients that don't and for better spam-filter scoring.
					Body = new Body
					{
						Html = new Content { Data = body.Html },
						Text = new Content { Data = body.Text },
					},
				},
			},
		}, ct);
}
