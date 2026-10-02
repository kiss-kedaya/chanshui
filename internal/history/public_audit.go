package history

import (
	"context"
	"fmt"
)

func (s *Store) AddPublicAudit(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history: store closed")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE public_audits SET total = total + 1 WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("history: public audit count: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("history: public audit counter missing")
	}
	return nil
}

func (s *Store) PublicAuditCount(ctx context.Context) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("history: store closed")
	}
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT total FROM public_audits WHERE id = 1`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("history: public audit count: %w", err)
	}
	return n, nil
}
