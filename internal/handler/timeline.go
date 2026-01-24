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

// ============================================
// ERRAND TIMELINE
// ============================================

type ErrandTimelineResponse struct {
	Steps       []TimelineStep `json:"steps"`
	CurrentStep string         `json:"current_step"`
	Description string         `json:"package_description"`
	DeliveryFee float64        `json:"delivery_fee"`
	DropoffCode *string        `json:"dropoff_code,omitempty"`
}

func BuildErrandTimeline(errand *domain.Errand) ErrandTimelineResponse {
	steps := []TimelineStep{}
	currentStep := ""

	// Step 1: Created
	steps = append(steps, TimelineStep{
		Step:        "created",
		Title:       "Errand Created",
		Description: "You created an errand request",
		Status:      "completed",
		Timestamp:   errand.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})

	// Step 2: Confirmed (Searching)
	confirmedStep := TimelineStep{
		Step:        "confirmed",
		Title:       "Searching for Rider",
		Description: "Looking for an available rider nearby",
		Status:      "pending",
	}

	// Step 3: Rider Accepted
	acceptedStep := TimelineStep{
		Step:        "accepted",
		Title:       "Rider Assigned",
		Description: "A rider has accepted your errand",
		Status:      "pending",
	}

	// Step 4: Picked Up
	pickedUpStep := TimelineStep{
		Step:        "picked_up",
		Title:       "Package Picked Up",
		Description: "Rider has collected the package",
		Status:      "pending",
	}

	// Step 5: In Transit
	inTransitStep := TimelineStep{
		Step:        "in_transit",
		Title:       "In Transit",
		Description: "Your package is on the way",
		Status:      "pending",
	}

	// Step 6: Delivered
	deliveredStep := TimelineStep{
		Step:        "delivered",
		Title:       "Delivered",
		Description: "Package has arrived. Confirm with your code.",
		Status:      "pending",
	}

	// Step 7: Completed
	completedStep := TimelineStep{
		Step:        "completed",
		Title:       "Completed",
		Description: "Errand completed successfully",
		Status:      "pending",
	}

	// Cancelled step (only shown when cancelled)
	var cancelledStep *TimelineStep

	switch errand.Status {
	case domain.ErrandStatusDraft:
		currentStep = "created"
		confirmedStep.Status = "pending"

	case domain.ErrandStatusSearching:
		confirmedStep.Status = "current"
		currentStep = "confirmed"
		if errand.ConfirmedAt != nil {
			confirmedStep.Timestamp = errand.ConfirmedAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.ErrandStatusAccepted:
		confirmedStep.Status = "completed"
		acceptedStep.Status = "current"
		currentStep = "accepted"
		if errand.AcceptedAt != nil {
			acceptedStep.Timestamp = errand.AcceptedAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.ErrandStatusPickedUp:
		confirmedStep.Status = "completed"
		acceptedStep.Status = "completed"
		pickedUpStep.Status = "current"
		currentStep = "picked_up"
		if errand.PickedUpAt != nil {
			pickedUpStep.Timestamp = errand.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.ErrandStatusInTransit:
		confirmedStep.Status = "completed"
		acceptedStep.Status = "completed"
		pickedUpStep.Status = "completed"
		inTransitStep.Status = "current"
		currentStep = "in_transit"

	case domain.ErrandStatusDelivered:
		confirmedStep.Status = "completed"
		acceptedStep.Status = "completed"
		pickedUpStep.Status = "completed"
		inTransitStep.Status = "completed"
		deliveredStep.Status = "current"
		deliveredStep.Description = "Package delivered. Enter your code to confirm."
		currentStep = "delivered"
		if errand.DeliveredAt != nil {
			deliveredStep.Timestamp = errand.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.ErrandStatusCompleted:
		confirmedStep.Status = "completed"
		acceptedStep.Status = "completed"
		pickedUpStep.Status = "completed"
		inTransitStep.Status = "completed"
		deliveredStep.Status = "completed"
		completedStep.Status = "completed"
		currentStep = "completed"
		if errand.CompletedAt != nil {
			completedStep.Timestamp = errand.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.ErrandStatusCancelled:
		cancelledStep = &TimelineStep{
			Step:        "cancelled",
			Title:       "Cancelled",
			Description: "Errand was cancelled",
			Status:      "completed",
		}
		if errand.CancelledAt != nil {
			cancelledStep.Timestamp = errand.CancelledAt.Format("2006-01-02T15:04:05Z07:00")
		}
		currentStep = "cancelled"
	}

	// Build final steps
	steps = append(steps, confirmedStep, acceptedStep, pickedUpStep, inTransitStep, deliveredStep)

	if cancelledStep != nil {
		steps = append(steps, *cancelledStep)
	} else {
		steps = append(steps, completedStep)
	}

	return ErrandTimelineResponse{
		Steps:       steps,
		CurrentStep: currentStep,
		Description: errand.PackageDescription,
		DeliveryFee: errand.DeliveryFee,
		DropoffCode: errand.DropoffCode,
	}
}

// ============================================
// DRIVER DELIVERY TIMELINE (Inter-state)
// ============================================

type DriverDeliveryTimelineResponse struct {
	Steps       []TimelineStep `json:"steps"`
	CurrentStep string         `json:"current_step"`
	ItemName    string         `json:"item_name"`
	TotalAmount float64        `json:"total_amount"`
	OriginCity  string         `json:"origin_city"`
	DestCity    string         `json:"destination_city"`
	PickupCode  *string        `json:"pickup_code,omitempty"`
	DropoffCode *string        `json:"dropoff_code,omitempty"`
}

func BuildDriverDeliveryTimeline(
	viewType TimelineViewType,
	delivery *domain.DriverDelivery,
	pr *domain.PaymentRequest,
) DriverDeliveryTimelineResponse {
	steps := []TimelineStep{}
	currentStep := ""

	// Step 1: Order Created
	orderDesc := "Order placed for inter-state delivery"
	if viewType == TimelineViewSeller {
		orderDesc = "You received an inter-state delivery order"
	}
	steps = append(steps, TimelineStep{
		Step:        "order_created",
		Title:       "Order Created",
		Description: orderDesc,
		Status:      "completed",
		Timestamp:   delivery.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})

	// Step 2: Awaiting Driver
	awaitingDriverStep := TimelineStep{
		Step:        "awaiting_driver",
		Title:       "Awaiting Driver",
		Description: "Searching for a driver traveling this route",
		Status:      "pending",
	}

	// Step 3: Driver Assigned
	driverAssignedStep := TimelineStep{
		Step:        "driver_assigned",
		Title:       "Driver Assigned",
		Description: "A driver has accepted your delivery",
		Status:      "pending",
	}

	// Step 4: Awaiting Pickup (seller brings to driver)
	awaitingPickupDesc := "Bring the item to the driver at the departure point"
	if viewType == TimelineViewBuyer {
		awaitingPickupDesc = "Seller is handing over item to driver"
	}
	awaitingPickupStep := TimelineStep{
		Step:        "awaiting_pickup",
		Title:       "Awaiting Handover",
		Description: awaitingPickupDesc,
		Status:      "pending",
	}

	// Step 5: Picked Up
	pickedUpStep := TimelineStep{
		Step:        "picked_up",
		Title:       "Item Collected",
		Description: "Driver has collected the item",
		Status:      "pending",
	}

	// Step 6: In Transit (Trip Departed)
	inTransitStep := TimelineStep{
		Step:        "in_transit",
		Title:       "In Transit",
		Description: "Item is on the way to destination city",
		Status:      "pending",
	}

	// Step 7: Arrived at Destination
	arrivedStep := TimelineStep{
		Step:        "arrived",
		Title:       "Arrived at Destination",
		Description: "Driver has arrived in destination city",
		Status:      "pending",
	}

	// Step 8: Delivered / Confirmation Window
	deliveredStep := TimelineStep{
		Step:        "delivered",
		Title:       "Delivered",
		Description: "Item delivered to buyer",
		Status:      "pending",
	}

	// Step 9: Completed (Payment Released)
	completedDesc := "Delivery completed successfully"
	if viewType == TimelineViewSeller {
		completedDesc = "Payment released to your wallet"
	}
	completedStep := TimelineStep{
		Step:        "completed",
		Title:       "Completed",
		Description: completedDesc,
		Status:      "pending",
	}

	// Edge case steps (only shown when applicable)
	var buyerUnreachableStep *TimelineStep
	var disputeStep *TimelineStep
	var returnStep *TimelineStep

	// Determine status based on delivery status
	switch delivery.Status {
	case domain.DDStatusAwaitingDriver:
		awaitingDriverStep.Status = "current"
		currentStep = "awaiting_driver"

	case domain.DDStatusDriverAssigned:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "current"
		currentStep = "driver_assigned"
		if delivery.DriverAssignedAt != nil {
			driverAssignedStep.Timestamp = delivery.DriverAssignedAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.DDStatusAwaitingPickup:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "completed"
		awaitingPickupStep.Status = "current"
		currentStep = "awaiting_pickup"
		if delivery.AtPickupAt != nil {
			awaitingPickupStep.Timestamp = delivery.AtPickupAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.DDStatusPickedUp:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "completed"
		awaitingPickupStep.Status = "completed"
		pickedUpStep.Status = "current"
		currentStep = "picked_up"
		if delivery.PickedUpAt != nil {
			pickedUpStep.Timestamp = delivery.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.DDStatusInTransit:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "completed"
		awaitingPickupStep.Status = "completed"
		pickedUpStep.Status = "completed"
		inTransitStep.Status = "current"
		currentStep = "in_transit"
		if delivery.DepartedAt != nil {
			inTransitStep.Timestamp = delivery.DepartedAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.DDStatusArrived:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "completed"
		awaitingPickupStep.Status = "completed"
		pickedUpStep.Status = "completed"
		inTransitStep.Status = "completed"
		arrivedStep.Status = "current"
		currentStep = "arrived"
		if delivery.ArrivedAt != nil {
			arrivedStep.Timestamp = delivery.ArrivedAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.DDStatusConfirmationWindow:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "completed"
		awaitingPickupStep.Status = "completed"
		pickedUpStep.Status = "completed"
		inTransitStep.Status = "completed"
		arrivedStep.Status = "completed"
		deliveredStep.Status = "current"
		deliveredStep.Description = "Awaiting buyer confirmation"
		currentStep = "delivered"
		if delivery.DeliveredAt != nil {
			deliveredStep.Timestamp = delivery.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.DDStatusCompleted, domain.DDStatusDriverPaid:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "completed"
		awaitingPickupStep.Status = "completed"
		pickedUpStep.Status = "completed"
		inTransitStep.Status = "completed"
		arrivedStep.Status = "completed"
		deliveredStep.Status = "completed"
		completedStep.Status = "completed"
		currentStep = "completed"
		if delivery.CompletedAt != nil {
			completedStep.Timestamp = delivery.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
		}

	case domain.DDStatusBuyerUnreachable:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "completed"
		awaitingPickupStep.Status = "completed"
		pickedUpStep.Status = "completed"
		inTransitStep.Status = "completed"
		arrivedStep.Status = "completed"
		// Add buyer unreachable step
		unreachableDesc := "Driver couldn't reach you. Please respond to avoid item return."
		if viewType == TimelineViewSeller {
			unreachableDesc = "Buyer was unreachable. Awaiting their response."
		}
		buyerUnreachableStep = &TimelineStep{
			Step:        "buyer_unreachable",
			Title:       "Buyer Unreachable",
			Description: unreachableDesc,
			Status:      "current",
		}
		if delivery.BuyerUnreachableAt != nil {
			buyerUnreachableStep.Timestamp = delivery.BuyerUnreachableAt.Format("2006-01-02T15:04:05Z07:00")
		}
		currentStep = "buyer_unreachable"

	case domain.DDStatusReturnToSeller:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "completed"
		awaitingPickupStep.Status = "completed"
		pickedUpStep.Status = "completed"
		inTransitStep.Status = "completed"
		arrivedStep.Status = "completed"
		buyerUnreachableStep = &TimelineStep{
			Step:        "buyer_unreachable",
			Title:       "Buyer Unreachable",
			Description: "Buyer did not respond in time",
			Status:      "completed",
		}
		returnDesc := "Item is being returned to seller"
		if viewType == TimelineViewSeller {
			returnDesc = "Item is being returned to you"
		}
		returnStep = &TimelineStep{
			Step:        "return_to_seller",
			Title:       "Returning to Seller",
			Description: returnDesc,
			Status:      "current",
		}
		currentStep = "return_to_seller"

	case domain.DDStatusReturnedToSeller:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "completed"
		awaitingPickupStep.Status = "completed"
		pickedUpStep.Status = "completed"
		inTransitStep.Status = "completed"
		arrivedStep.Status = "completed"
		buyerUnreachableStep = &TimelineStep{
			Step:        "buyer_unreachable",
			Title:       "Buyer Unreachable",
			Description: "Buyer did not respond",
			Status:      "completed",
		}
		returnStep = &TimelineStep{
			Step:        "returned_to_seller",
			Title:       "Returned to Seller",
			Description: "Item returned successfully",
			Status:      "completed",
		}
		currentStep = "returned_to_seller"

	case domain.DDStatusDisputed:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "completed"
		awaitingPickupStep.Status = "completed"
		pickedUpStep.Status = "completed"
		inTransitStep.Status = "completed"
		arrivedStep.Status = "completed"
		deliveredStep.Status = "completed"
		disputeDesc := "You opened a dispute. Our team is reviewing."
		if viewType == TimelineViewSeller {
			disputeDesc = "Buyer opened a dispute. Our team is reviewing."
		}
		disputeStep = &TimelineStep{
			Step:        "disputed",
			Title:       "Dispute Opened",
			Description: disputeDesc,
			Status:      "current",
		}
		currentStep = "disputed"

	case domain.DDStatusRefunded:
		awaitingDriverStep.Status = "completed"
		driverAssignedStep.Status = "completed"
		refundedDesc := "Payment has been refunded"
		completedStep = TimelineStep{
			Step:        "refunded",
			Title:       "Refunded",
			Description: refundedDesc,
			Status:      "completed",
		}
		currentStep = "refunded"

	case domain.DDStatusCancelled:
		cancelledDesc := "Delivery was cancelled"
		completedStep = TimelineStep{
			Step:        "cancelled",
			Title:       "Cancelled",
			Description: cancelledDesc,
			Status:      "completed",
		}
		currentStep = "cancelled"

	case domain.DDStatusExpired:
		awaitingDriverStep.Status = "completed"
		expiredDesc := "No driver found. Delivery expired."
		completedStep = TimelineStep{
			Step:        "expired",
			Title:       "Expired",
			Description: expiredDesc,
			Status:      "completed",
		}
		currentStep = "expired"
	}

	// Build final steps array
	steps = append(steps, awaitingDriverStep, driverAssignedStep, awaitingPickupStep,
		pickedUpStep, inTransitStep, arrivedStep)

	// Add edge case steps if applicable
	if buyerUnreachableStep != nil {
		steps = append(steps, *buyerUnreachableStep)
	}
	if returnStep != nil {
		steps = append(steps, *returnStep)
	}
	if disputeStep != nil {
		steps = append(steps, *disputeStep)
	}

	// Add final step (delivered/completed or terminal state)
	if delivery.Status != domain.DDStatusBuyerUnreachable &&
		delivery.Status != domain.DDStatusReturnToSeller &&
		delivery.Status != domain.DDStatusReturnedToSeller &&
		delivery.Status != domain.DDStatusDisputed {
		steps = append(steps, deliveredStep, completedStep)
	}

	resp := DriverDeliveryTimelineResponse{
		Steps:       steps,
		CurrentStep: currentStep,
		ItemName:    pr.ItemName,
		TotalAmount: delivery.ItemValue + delivery.DriverFee,
		OriginCity:  delivery.OriginCity,
		DestCity:    delivery.DestinationCity,
	}

	// Add codes based on view type
	if viewType == TimelineViewSeller && delivery.PickupCode != nil {
		resp.PickupCode = delivery.PickupCode
	}
	if viewType == TimelineViewBuyer && delivery.DropoffCode != nil {
		resp.DropoffCode = delivery.DropoffCode
	}

	return resp
}
