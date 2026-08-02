package account

import "testing"

// TestCollectionPaths pins the three Firestore path variants the legacy backend
// resolves at runtime. A wrong path here reads or writes another user's
// financial data, so each variant is asserted explicitly.
//
// The paths mirror accountService.ts. Note the asymmetry the legacy schema
// carries: the legacy variant stores plans under "simulations", while the
// private and shared variants use "plans".
func TestCollectionPaths(t *testing.T) {
	tests := []struct {
		name           string
		ctx            *Context
		wantCategories string
		wantTx         string
		wantPlans      string
	}{
		{
			name:           "legacy",
			ctx:            &Context{UserID: "u1", UsesLegacyPaths: true},
			wantCategories: "users/u1/categories",
			wantTx:         "users/u1/transactions",
			wantPlans:      "users/u1/simulations",
		},
		{
			name:           "private",
			ctx:            &Context{UserID: "u1", AccountID: "a1"},
			wantCategories: "users/u1/accounts/a1/categories",
			wantTx:         "users/u1/accounts/a1/transactions",
			wantPlans:      "users/u1/accounts/a1/plans",
		},
		{
			name:           "shared",
			ctx:            &Context{UserID: "u1", AccountID: "a1", SharedAccountID: "s1"},
			wantCategories: "sharedAccounts/s1/categories",
			wantTx:         "sharedAccounts/s1/transactions",
			wantPlans:      "sharedAccounts/s1/plans",
		},
		{
			name:           "shared_takes_precedence_over_private",
			ctx:            &Context{UserID: "u1", AccountID: "a1", SharedAccountID: "s1", Role: "MEMBER"},
			wantCategories: "sharedAccounts/s1/categories",
			wantTx:         "sharedAccounts/s1/transactions",
			wantPlans:      "sharedAccounts/s1/plans",
		},
		{
			name:           "legacy_wins_even_when_ids_present",
			ctx:            &Context{UserID: "u1", AccountID: "a1", SharedAccountID: "s1", UsesLegacyPaths: true},
			wantCategories: "users/u1/categories",
			wantTx:         "users/u1/transactions",
			wantPlans:      "users/u1/simulations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.CategoriesPath(); got != tt.wantCategories {
				t.Errorf("CategoriesPath() = %q, want %q", got, tt.wantCategories)
			}
			if got := tt.ctx.TransactionsPath(); got != tt.wantTx {
				t.Errorf("TransactionsPath() = %q, want %q", got, tt.wantTx)
			}
			if got := tt.ctx.PlansPath(); got != tt.wantPlans {
				t.Errorf("PlansPath() = %q, want %q", got, tt.wantPlans)
			}
		})
	}
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		name string
		ctx  *Context
		want string
	}{
		{name: "legacy", ctx: &Context{UserID: "u1", UsesLegacyPaths: true}, want: "u1:legacy"},
		{name: "private", ctx: &Context{UserID: "u1", AccountID: "a1"}, want: "u1:a1"},
		{name: "shared", ctx: &Context{UserID: "u1", AccountID: "a1", SharedAccountID: "s1"}, want: "u1:a1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.CacheKey(); got != tt.want {
				t.Errorf("CacheKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultCategories(t *testing.T) {
	cats := DefaultCategories()
	if len(cats) != 6 {
		t.Fatalf("DefaultCategories() returned %d categories, want 6", len(cats))
	}

	var income, expense int
	for _, c := range cats {
		switch c.Type {
		case "INCOME":
			income++
		case "EXPENSE":
			expense++
		default:
			t.Errorf("category %q has unexpected type %q", c.Name, c.Type)
		}
		if c.ID == "" || c.Name == "" || c.Icon == "" || c.Color == "" {
			t.Errorf("category %+v has an empty required field", c)
		}
	}

	if income != 2 {
		t.Errorf("got %d INCOME categories, want 2", income)
	}
	if expense != 4 {
		t.Errorf("got %d EXPENSE categories, want 4", expense)
	}
}

// TestCanMutateRecord pins the shared-workspace permission model. Getting this
// wrong lets a member edit another member's financial records.
func TestCanMutateRecord(t *testing.T) {
	tests := []struct {
		name        string
		ctx         *Context
		recordOwner string
		want        bool
	}{
		{
			name:        "private_account_always_allowed",
			ctx:         &Context{UserID: "u1", AccountID: "a1"},
			recordOwner: "someone-else",
			want:        true,
		},
		{
			name:        "legacy_account_always_allowed",
			ctx:         &Context{UserID: "u1", UsesLegacyPaths: true},
			recordOwner: "someone-else",
			want:        true,
		},
		{
			name:        "shared_owner_can_edit_others",
			ctx:         &Context{UserID: "u1", AccountID: "a1", SharedAccountID: "s1", Role: "OWNER"},
			recordOwner: "u2",
			want:        true,
		},
		{
			name:        "shared_member_can_edit_own",
			ctx:         &Context{UserID: "u2", AccountID: "a1", SharedAccountID: "s1", Role: "MEMBER"},
			recordOwner: "u2",
			want:        true,
		},
		{
			name:        "shared_member_cannot_edit_others",
			ctx:         &Context{UserID: "u2", AccountID: "a1", SharedAccountID: "s1", Role: "MEMBER"},
			recordOwner: "u3",
			want:        false,
		},
		{
			name:        "shared_member_cannot_edit_legacy_record",
			ctx:         &Context{UserID: "u2", AccountID: "a1", SharedAccountID: "s1", Role: "MEMBER"},
			recordOwner: "",
			want:        false,
		},
		{
			name:        "shared_owner_can_edit_legacy_record",
			ctx:         &Context{UserID: "u1", AccountID: "a1", SharedAccountID: "s1", Role: "OWNER"},
			recordOwner: "",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.CanMutateRecord(tt.recordOwner); got != tt.want {
				t.Errorf("CanMutateRecord(%q) = %v, want %v", tt.recordOwner, got, tt.want)
			}
		})
	}
}
