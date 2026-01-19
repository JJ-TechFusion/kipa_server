package handler

import (
	"kipa/internal/domain"
)

type TimelineStep struct {
	Step        string `json:"step"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"` // "completed", "current", "pending"
	Timestamp   string `json:"timestamp,omitempty"`
}

type TimelineResponse struct {
	Steps        []TimelineStep `json:"steps"`
	CurrentStep  string         `json:"current_step"`
	ItemName     string         `json:"item_name"`
	TotalAmount  float64        `json:"total_amount"`
	DeliveryType string         `json:"delivery_type"`
	PickupCode   *string        `json:"pickup_code,omitempty"`  // For seller
	DropoffCode  *string        `json:"dropoff_code,omitempty"` // For buyer
}

type TimelineViewType string

const (
	TimelineViewBuyer  TimelineViewType = "buyer"
	TimelineViewSeller TimelineViewType = "seller"
)

func BuildTimeline(
	viewType TimelineViewType,
	pr *domain.PaymentRequest,
	order *domain.Order,
	job *domain.DeliveryJob,
) TimelineResponse {
	steps := []TimelineStep{}
	currentStep := ""

	// Step 1: Link Created
	linkDesc := "Seller created a payment request"
	if viewType == TimelineViewSeller {
		linkDesc = "You created a payment request"
	}
	steps = append(steps, TimelineStep{
		Step:        "link_created",
		Title:       "Payment Link Created",
		Description: linkDesc,
		Status:      "completed",
		Timestamp:   pr.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})

	// Step 2: Payment Received
	paymentDesc := "Your payment is secured in escrow"
	if viewType == TimelineViewSeller {
		paymentDesc = "Buyer's payment is secured in escrow"
	}
	paymentStep := TimelineStep{
		Step:        "payment_received",
		Title:       "Payment Received",
		Description: paymentDesc,
		Status:      "pending",
	}
	if order.PaymentStatus == domain.OrderPaymentVerified {
		paymentStep.Status = "completed"
		if order.PaidAt != nil {
			paymentStep.Timestamp = order.PaidAt.Format("2006-01-02T15:04:05Z07:00")
		}
	} else {
		paymentStep.Status = "current"
		currentStep = "payment_received"
	}
	steps = append(steps, paymentStep)

	if pr.DeliveryType == domain.DeliveryTypeIntraState {
		steps, currentStep = appendDeliverySteps(steps, currentStep, viewType, order, job)
	} else {
		steps, currentStep = appendSimpleSteps(steps, currentStep, viewType, order)
	}

	resp := TimelineResponse{
		Steps:        steps,
		CurrentStep:  currentStep,
		ItemName:     pr.ItemName,
		TotalAmount:  order.TotalAmount,
		DeliveryType: string(pr.DeliveryType),
	}

	if job != nil && job.RiderID != nil {
		if viewType == TimelineViewSeller {
			resp.PickupCode = job.PickupCode
		} else {
			resp.DropoffCode = job.DropoffCode
		}
	}

	return resp
}

func appendDeliverySteps(
	steps []TimelineStep,
	currentStep string,
	viewType TimelineViewType,
	order *domain.Order,
	job *domain.DeliveryJob,
) ([]TimelineStep, string) {

	awaitingTitle := "Awaiting Rider Assignment"
	awaitingDesc := "Searching for a delivery rider"
	if viewType == TimelineViewSeller {
		awaitingTitle = "Ready for Pickup"
		awaitingDesc = "Mark item as ready to trigger rider search"
	}
	awaitingStep := TimelineStep{
		Step:        "awaiting_rider",
		Title:       awaitingTitle,
		Description: awaitingDesc,
		Status:      "pending",
	}

	riderDesc := "A rider has been assigned to your delivery"
	if viewType == TimelineViewSeller {
		riderDesc = "A rider is on the way to collect the item"
	}
	riderStep := TimelineStep{
		Step:        "rider_assigned",
		Title:       "Rider Assigned",
		Description: riderDesc,
		Status:      "pending",
	}

	pickedUpDesc := "Rider has collected the item from seller"
	if viewType == TimelineViewSeller {
		pickedUpDesc = "Rider has collected the item"
	}
	pickedUpStep := TimelineStep{
		Step:        "picked_up",
		Title:       "Item Picked Up",
		Description: pickedUpDesc,
		Status:      "pending",
	}

	inTransitStep := TimelineStep{
		Step:        "in_transit",
		Title:       "In Transit",
		Description: "Your item is on the way",
		Status:      "pending",
	}

	deliveredStep := TimelineStep{
		Step:        "delivered",
		Title:       "Delivered",
		Description: "Item has been delivered successfully",
		Status:      "pending",
	}

	var releasedStep *TimelineStep
	if viewType == TimelineViewSeller {
		releasedStep = &TimelineStep{
			Step:        "payment_released",
			Title:       "Payment Released",
			Description: "Funds released to your wallet",
			Status:      "pending",
		}
	}

	if job != nil {
		switch job.Status {
		case domain.DJStatusSearching:
			awaitingStep.Status = "current"
			currentStep = "awaiting_rider"
			if job.SearchStartedAt != nil {
				awaitingStep.Timestamp = job.SearchStartedAt.Format("2006-01-02T15:04:05Z07:00")
			}

		case domain.DJStatusAccepted, domain.DJStatusEnRoutePickup:
			awaitingStep.Status = "completed"
			riderStep.Status = "current"
			currentStep = "rider_assigned"
			if job.AcceptedAt != nil {
				riderStep.Timestamp = job.AcceptedAt.Format("2006-01-02T15:04:05Z07:00")
			}

		case domain.DJStatusPickedUp:
			awaitingStep.Status = "completed"
			riderStep.Status = "completed"
			pickedUpStep.Status = "current"
			currentStep = "picked_up"
			if job.PickedUpAt != nil {
				pickedUpStep.Timestamp = job.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
			}

		case domain.DJStatusInTransit:
			awaitingStep.Status = "completed"
			riderStep.Status = "completed"
			pickedUpStep.Status = "completed"
			inTransitStep.Status = "current"
			currentStep = "in_transit"
			if job.PickedUpAt != nil {
				pickedUpStep.Timestamp = job.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
			}

		case domain.DJStatusDelivered:
			awaitingStep.Status = "completed"
			riderStep.Status = "completed"
			pickedUpStep.Status = "completed"
			inTransitStep.Status = "completed"
			deliveredStep.Status = "completed"
			currentStep = "delivered"
			if job.DeliveredAt != nil {
				deliveredStep.Timestamp = job.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
			}
			if releasedStep != nil {
				releasedStep.Status = "current"
				currentStep = "payment_released"
			}

		case domain.DJStatusPaidOut:
			awaitingStep.Status = "completed"
			riderStep.Status = "completed"
			pickedUpStep.Status = "completed"
			inTransitStep.Status = "completed"
			deliveredStep.Status = "completed"
			if releasedStep != nil {
				releasedStep.Status = "completed"
			}
			currentStep = "delivered"
			if releasedStep != nil {
				currentStep = "payment_released"
			}
		}
	} else if order.PaymentStatus == domain.OrderPaymentVerified {
		// Payment done but no job yet
		if viewType == TimelineViewSeller {
			awaitingStep.Status = "current"
			currentStep = "awaiting_rider"
		} else {
			awaitingStep.Title = "Awaiting Seller Preparation"
			awaitingStep.Description = "Waiting for seller to prepare item for delivery"
			awaitingStep.Status = "current"
			currentStep = "awaiting_rider"
		}
	}

	steps = append(steps, awaitingStep, riderStep, pickedUpStep, inTransitStep, deliveredStep)
	if releasedStep != nil {
		steps = append(steps, *releasedStep)
	}

	return steps, currentStep
}

func appendSimpleSteps(
	steps []TimelineStep,
	currentStep string,
	viewType TimelineViewType,
	order *domain.Order,
) ([]TimelineStep, string) {

	fulfilledDesc := "Seller has completed the order"
	if viewType == TimelineViewSeller {
		fulfilledDesc = "Complete the order to release payment"
	}
	fulfilledStep := TimelineStep{
		Step:        "fulfilled",
		Title:       "Order Fulfilled",
		Description: fulfilledDesc,
		Status:      "pending",
	}

	var releasedStep *TimelineStep
	if viewType == TimelineViewSeller {
		releasedStep = &TimelineStep{
			Step:        "payment_released",
			Title:       "Payment Released",
			Description: "Funds released to your wallet",
			Status:      "pending",
		}
	}

	if order.Status == domain.OrderStatusCompleted {
		fulfilledStep.Status = "completed"
		currentStep = "fulfilled"
		if releasedStep != nil {
			releasedStep.Status = "completed"
			currentStep = "payment_released"
		}
	} else if order.PaymentStatus == domain.OrderPaymentVerified {
		fulfilledStep.Status = "current"
		currentStep = "fulfilled"
	}

	steps = append(steps, fulfilledStep)
	if releasedStep != nil {
		steps = append(steps, *releasedStep)
	}

	return steps, currentStep
}
