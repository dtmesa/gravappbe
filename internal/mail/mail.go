// Package mail sends the account emails: username reminders, password-reset
// codes and email confirmations.
package mail

import (
	"context"
	"log/slog"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// Body pairs the HTML and plain-text parts so neither can be sent alone.
type Body struct {
	HTML string
	Text string
}

type Sender interface {
	Send(ctx context.Context, to, subject string, body Body) error
}

// Console logs emails instead of sending them, so the whole recovery and
// confirmation flow -- including reading the code -- works locally with no AWS
// credentials. It logs the code in plain text, which is why it is never the
// default inside Lambda.
type Console struct{}

func (Console) Send(ctx context.Context, to, subject string, body Body) error {
	slog.InfoContext(ctx, "[email]", "to", to, "subject", subject, "body", body.Text)
	return nil
}

// SES sends through SES v2. A new AWS account's SES starts in the sandbox,
// which only delivers to verified addresses -- a send to anyone else fails.
type SES struct {
	client *sesv2.Client
	from   string
}

func NewSES(client *sesv2.Client, from string) *SES {
	return &SES{client: client, from: from}
}

func (s *SES) Send(ctx context.Context, to, subject string, body Body) error {
	_, err := s.client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(s.from),
		Destination:      &types.Destination{ToAddresses: []string{to}},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: aws.String(subject)},
				// multipart/alternative: HTML where it renders, text as the
				// fallback and for spam-filter scoring.
				Body: &types.Body{
					Html: &types.Content{Data: aws.String(body.HTML)},
					Text: &types.Content{Data: aws.String(body.Text)},
				},
			},
		},
	})

	return err
}

// Message is one email captured by a Recorder.
type Message struct {
	To, Subject string
	Body        Body
}

// Recorder keeps every email in memory, for tests.
type Recorder struct {
	mu   sync.Mutex
	sent []Message
	fail error
}

// SetFail makes subsequent sends return err (nil restores success), to
// exercise send-failure paths.
func (r *Recorder) SetFail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fail = err
}

func (r *Recorder) Send(_ context.Context, to, subject string, body Body) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.fail != nil {
		return r.fail
	}

	r.sent = append(r.sent, Message{To: to, Subject: subject, Body: body})

	return nil
}

// Last returns the most recent email sent to the address.
func (r *Recorder) Last(to string) (Message, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := len(r.sent) - 1; i >= 0; i-- {
		if r.sent[i].To == to {
			return r.sent[i], true
		}
	}

	return Message{}, false
}

// Count returns how many emails were sent to the address.
func (r *Recorder) Count(to string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := 0
	for _, m := range r.sent {
		if m.To == to {
			n++
		}
	}

	return n
}
