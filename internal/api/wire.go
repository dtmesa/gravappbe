package api

import (
	"context"
	"net/http"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"

	"github.com/dtmesa/gravappbe/internal/auth"
	"github.com/dtmesa/gravappbe/internal/config"
	"github.com/dtmesa/gravappbe/internal/dynamo"
	"github.com/dtmesa/gravappbe/internal/mail"
	"github.com/dtmesa/gravappbe/internal/ratelimit"
	"github.com/dtmesa/gravappbe/internal/store"
)

// Wire builds the service from configuration. Both entrypoints call it once,
// before serving -- in Lambda that is the init phase, so none of this repeats
// per request.
func Wire(ctx context.Context, cfg config.Config) (http.Handler, *dynamo.DB, error) {
	db, err := dynamo.Connect(ctx, cfg.Region, cfg.DynamoEndpoint, cfg.TablePrefix)
	if err != nil {
		return nil, nil, err
	}

	var sender mail.Sender = mail.Console{}

	if cfg.EmailSender == "ses" {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithRegion(cfg.Region),
			awsconfig.WithEC2IMDSClientEnableState(imds.ClientDisabled),
		)
		if err != nil {
			return nil, nil, err
		}

		sender = mail.NewSES(sesv2.NewFromConfig(awsCfg), cfg.EmailFrom)
	}

	now := time.Now

	return New(Deps{
		Config:  cfg,
		Store:   store.New(db, now),
		Signer:  auth.NewSigner(cfg.JWTSecret, now),
		Limiter: ratelimit.New(db, now, cfg.RateLimits.Enabled),
		Mail:    sender,
		Now:     now,
	}), db, nil
}
