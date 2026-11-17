package messaging

import (
	"errors"
	"testing"

	"wallet-svc/pkg/response"

	"github.com/stretchr/testify/require"
)

func TestClassifyProcessError(t *testing.T) {
	require.Equal(t, SettleAccept, ClassifyProcessError(nil))
	require.Equal(t, SettleDeadLetter, ClassifyProcessError(response.Unprocessable(response.CodeInsufficientFunds, "no funds")))
	require.Equal(t, SettleDeadLetter, ClassifyProcessError(response.Conflict(response.CodeKeyPayloadMismatch, "mismatch")))
	require.Equal(t, SettleRelease, ClassifyProcessError(response.ServiceUnavailable(response.CodeLockTimeout, "busy")))
	require.Equal(t, SettleRelease, ClassifyProcessError(errors.New("dial tcp timeout")))
}
