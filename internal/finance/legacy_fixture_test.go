package finance

import (
	"context"
	"github.com/Faust217/LifeOS/internal/identity"
)

// Old regression fixtures represent pre-v8 accounts; production creation never uses this path.
func legacyTestAccount(ctx context.Context, s *Service, owner identity.OwnerID, in AccountInput) (Account, error) {
	id, err := newUUID()
	if err != nil {
		return Account{}, err
	}
	_, err = s.repo.pool.Exec(ctx, `INSERT INTO finance_accounts(id,owner_id,name,account_type,currency,opening_balance_minor) VALUES($1,$2,$3,$4,$5,$6)`, id, owner.String(), in.Name, in.Type, in.Currency, in.OpeningBalanceMinor)
	return Account{ID: id, Name: in.Name, Type: in.Type, Currency: in.Currency, OpeningBalanceMinor: in.OpeningBalanceMinor}, err
}
