// operation.go — the postmark: the canonical vocabulary for what a call does,
// shared by every provider so one query spans them all.

package wirelog

// Canonical operations. Providers label each outbound call with one of these
// so `operation` means the same thing across every provider.
const (
	// OperationBalanceCheck reads available balance or float.
	OperationBalanceCheck Operation = "balance_check"
	// OperationPayout sends money out to a beneficiary.
	OperationPayout Operation = "payout"
	// OperationCollection pulls money in from a payer.
	OperationCollection Operation = "collection"
	// OperationRefund reverses a previous payment.
	OperationRefund Operation = "refund"
	// OperationAccountVerification validates a beneficiary account before use.
	OperationAccountVerification Operation = "account_verification"
	// OperationStatusCheck looks up the state of an earlier transaction.
	OperationStatusCheck Operation = "status_check"
	// OperationTransactionHistory lists past transactions.
	OperationTransactionHistory Operation = "transaction_history"
	// OperationMethodsLookup lists supported payment methods, banks or corridors.
	OperationMethodsLookup Operation = "methods_lookup"
	// OperationExchangeRate quotes an FX rate.
	OperationExchangeRate Operation = "exchange_rate"
)
