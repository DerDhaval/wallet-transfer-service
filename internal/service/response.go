package service

import (
	"encoding/json"
	"time"

	"wallet-transfer-service/internal/domain"
)

type transferJSON struct {
	TransferID    string    `json:"transferId"`
	FromWalletID  string    `json:"fromWalletId"`
	ToWalletID    string    `json:"toWalletId"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	FailureReason *string   `json:"failureReason,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func processedBody(t domain.Transfer) ([]byte, error) {
	return marshalJSON(toTransferJSON(t))
}

func failedBody(t domain.Transfer, cause *domain.Error) ([]byte, error) {
	return marshalJSON(map[string]any{
		"error": map[string]string{
			"code":    cause.Code,
			"message": cause.Message,
		},
		"transfer": toTransferJSON(t),
	})
}

func toTransferJSON(t domain.Transfer) transferJSON {
	return transferJSON{
		TransferID:    t.ID,
		FromWalletID:  t.FromWalletID,
		ToWalletID:    t.ToWalletID,
		Amount:        t.Amount,
		Currency:      t.Currency,
		Status:        string(t.Status),
		FailureReason: t.FailureReason,
		CreatedAt:     t.CreatedAt.UTC(),
		UpdatedAt:     t.UpdatedAt.UTC(),
	}
}

func marshalJSON(v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}
