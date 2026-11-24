package handler

import (
	"net/http"
	"strconv"
	"strings"

	"wallet-transfer-service/internal/domain"
)

// Currencies lists every supported ISO 4217 currency.
func (h *Handler) Currencies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"currencies": domain.Currencies()})
}

// Convert previews a conversion without moving money.
// GET /convert?from=INR&to=USD&amount=10000 (amount in minor units of "from").
func (h *Handler) Convert(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from := strings.ToUpper(strings.TrimSpace(q.Get("from")))
	to := strings.ToUpper(strings.TrimSpace(q.Get("to")))
	if err := requireCurrencyParam("from", from); err != nil {
		writeError(w, r, err)
		return
	}
	if err := requireCurrencyParam("to", to); err != nil {
		writeError(w, r, err)
		return
	}

	amount, err := strconv.ParseInt(q.Get("amount"), 10, 64)
	if err != nil || amount <= 0 || amount > domain.MaxTransferAmount {
		writeError(w, r, domain.ErrInvalidRequest.WithMessage(
			"Query parameter \"amount\" must be a whole number of minor units between 1 and 1000000000000"))
		return
	}

	conversion, err := domain.Convert(from, to, amount)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, conversion)
}

func requireCurrencyParam(name, code string) error {
	if _, ok := domain.LookupCurrency(code); ok {
		return nil
	}
	return domain.ErrInvalidRequest.WithMessage(
		"Query parameter \"" + name + "\" must be a supported currency code such as INR or USD. Use GET /currencies to see all of them")
}
