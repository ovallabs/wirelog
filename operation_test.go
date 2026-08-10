package wirelog

import "testing"

// TestOperationWireValues pins the stored strings. They are written to the
// operation column and queried by the dashboard, so renaming one silently
// splits a provider's history in two.
func TestOperationWireValues(t *testing.T) {
	for _, tc := range []struct {
		op   Operation
		want string
	}{
		{OperationBalanceCheck, "balance_check"},
		{OperationPayout, "payout"},
		{OperationCollection, "collection"},
		{OperationRefund, "refund"},
		{OperationAccountVerification, "account_verification"},
		{OperationStatusCheck, "status_check"},
		{OperationTransactionHistory, "transaction_history"},
		{OperationMethodsLookup, "methods_lookup"},
		{OperationExchangeRate, "exchange_rate"},
		{OperationAuthentication, "authentication"},
	} {
		if string(tc.op) != tc.want {
			t.Errorf("Operation = %q, want %q", tc.op, tc.want)
		}
	}
}
