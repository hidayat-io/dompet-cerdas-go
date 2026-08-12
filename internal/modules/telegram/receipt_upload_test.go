package telegram

import (
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/modules/account"
)

// The path must match storage.rules, which is what lets the web app resolve a
// display URL from it; the legacy flat path is deliberately not used.
func TestReceiptStoragePath(t *testing.T) {
	tests := []struct {
		name string
		ac   *account.Context
		want string
	}{
		{
			name: "private_account",
			ac:   &account.Context{UserID: "u1", AccountID: "a1"},
			want: "users/u1/accounts/a1/attachments/receipt_1.jpg",
		},
		{
			name: "shared_account_wins",
			ac:   &account.Context{UserID: "u1", AccountID: "a1", SharedAccountID: "s1"},
			want: "sharedAccounts/s1/attachments/receipt_1.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := receiptStoragePath(tt.ac, "receipt_1.jpg"); got != tt.want {
				t.Errorf("receiptStoragePath = %q, want %q", got, tt.want)
			}
		})
	}
}
