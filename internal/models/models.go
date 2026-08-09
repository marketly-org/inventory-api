package models

// Product represents an inventory item.
type Product struct {
	SKU        string `json:"sku"`
	Name       string `json:"name"`
	Stock      int    `json:"stock"`
	Reserved   int    `json:"reserved"`
	PriceCents int    `json:"price_cents"`
}

// Available returns the stock available for new orders.
func (p *Product) Available() int {
	return p.Stock - p.Reserved
}

// ReserveRequest is the body for POST /reserve.
type ReserveRequest struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

// ConfirmRequest is the body for POST /confirm.
type ConfirmRequest struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}
