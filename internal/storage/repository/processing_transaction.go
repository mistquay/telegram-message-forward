package repository

import (
	"context"
	"gorm.io/gorm"
	"telegram-message-forward/internal/domain/decision"
	domaindelivery "telegram-message-forward/internal/domain/delivery"
	domainsink "telegram-message-forward/internal/domain/sink"
)

// ProcessingTransaction commits delivery tasks and their decision evidence together.
type ProcessingTransaction struct{ db *gorm.DB }

func NewProcessingTransaction(db *gorm.DB) *ProcessingTransaction {
	return &ProcessingTransaction{db: db}
}

func (t *ProcessingTransaction) Within(ctx context.Context, fn func(domaindelivery.Repository, decision.Repository, domainsink.RoutingReader) error) error {
	return t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(NewDeliveryRepository(tx), NewDecisionRepository(tx), NewSinkRepository(tx, nil))
	})
}
