package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"kipa/internal/domain"
	"kipa/internal/repository"
)
type OrderRepo struct {
	db *sqlx.DB
}
func NewOrderRepo(db *sqlx.DB) repository.OrderRepository {
	return &OrderRepo{db: db}
}

func (r *OrderRepo) Create(ctx context.Context, order *domain.Order) error {
	query := `
		INSERT INTO orders (
			id, payment_request_id, buyer_id, escrow_id,
			payment_method, paystack_reference, paystack_access_code, payment_status,
			item_amount, delivery_fee_estimate, buyer_service_fee, total_amount,
			status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING created_at, updated_at`

	if order.ID == uuid.Nil {
		order.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		order.ID,
		order.PaymentRequestID,
		order.BuyerID,
		order.EscrowID,
		order.PaymentMethod,
		order.PaystackReference,
		order.PaystackAccessCode,
		order.PaymentStatus,
		order.ItemAmount,
		order.DeliveryFeeEstimate,
		order.BuyerServiceFee,
		order.TotalAmount,
		order.Status,
	).Scan(&order.CreatedAt, &order.UpdatedAt)
}

func (r *OrderRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	var order domain.Order
	query := `SELECT * FROM orders WHERE id = $1`

	err := r.db.GetContext(ctx, &order, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrOrderNotFound
		}
		return nil, err
	}

	return &order, nil
}

func (r *OrderRepo) GetByPaystackReference(ctx context.Context, reference string) (*domain.Order, error) {
	var order domain.Order
	query := `SELECT * FROM orders WHERE paystack_reference = $1`

	err := r.db.GetContext(ctx, &order, query, reference)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrOrderNotFound
		}
		return nil, err
	}

	return &order, nil
}

func (r *OrderRepo) GetByPaymentRequestID(ctx context.Context, prID uuid.UUID, limit, offset int) ([]*domain.Order, error) {
	var orders []*domain.Order
	query := `SELECT * FROM orders WHERE payment_request_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &orders, query, prID, limit, offset)
	if err != nil {
		return nil, err
	}

	return orders, nil
}

func (r *OrderRepo) GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.Order, error) {
	var orders []*domain.Order
	query := `SELECT * FROM orders WHERE buyer_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &orders, query, buyerID, limit, offset)
	if err != nil {
		return nil, err
	}

	return orders, nil
}

func (r *OrderRepo) GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.Order, error) {
	var orders []*domain.Order
	query := `
		SELECT o.* FROM orders o
		JOIN payment_requests pr ON o.payment_request_id = pr.id
		WHERE pr.seller_id = $1
		ORDER BY o.created_at DESC
		LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &orders, query, sellerID, limit, offset)
	if err != nil {
		return nil, err
	}

	return orders, nil
}

func (r *OrderRepo) Update(ctx context.Context, order *domain.Order) error {
	query := `
		UPDATE orders SET
			escrow_id = $2,
			paystack_reference = $3,
			paystack_access_code = $4,
			payment_status = $5,
			status = $6,
			paid_at = $7,
			updated_at = NOW()
		WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query,
		order.ID,
		order.EscrowID,
		order.PaystackReference,
		order.PaystackAccessCode,
		order.PaymentStatus,
		order.Status,
		order.PaidAt,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrOrderNotFound
	}

	return nil
}

func (r *OrderRepo) UpdatePaymentStatus(ctx context.Context, id uuid.UUID, status domain.OrderPaymentStatus) error {
	query := `UPDATE orders SET payment_status = $2, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrOrderNotFound
	}

	return nil
}

func (r *OrderRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error {
	query := `UPDATE orders SET status = $2, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrOrderNotFound
	}

	return nil
}

func (r *OrderRepo) SetEscrowID(ctx context.Context, id uuid.UUID, escrowID uuid.UUID) error {
	query := `UPDATE orders SET escrow_id = $2, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, escrowID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrOrderNotFound
	}

	return nil
}
