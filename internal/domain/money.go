package domain

// Money is an integer count of minor units (paise, cents). Never use floats.

const (
	// MaxTransferAmount is a guard against accidental huge postings.
	// 10^12 minor units is 10 billion in a 2-decimal currency.
	MaxTransferAmount int64 = 1_000_000_000_000

	// MaxOpeningBalance is the largest opening balance accepted by the API.
	MaxOpeningBalance int64 = 1_000_000_000_000_000
)
