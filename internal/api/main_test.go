package api_test

import (
	"testing"

	"github.com/dtmesa/gravappbe/internal/testutil"
)

// Integration tests run the real API against DynamoDB Local:
//
//	docker run -d -p 8000:8000 amazon/dynamodb-local
//	DYNAMODB_ENDPOINT=http://localhost:8000 go test ./...
//
// Without DYNAMODB_ENDPOINT they skip.
func TestMain(m *testing.M) { testutil.Main(m) }
