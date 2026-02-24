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
	Phase        string         `json:"phase"`                  // "delivery" or "return"
	PickupCode   *string        `json:"pickup_code,omitempty"`  // For seller (delivery) or buyer (return)
	DropoffCode  *string        `json:"dropoff_code,omitempty"` // For buyer (delivery) or seller (return)
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
	return BuildTimelineWithReturn(viewType, pr, order, job, nil)
}

func BuildTimelineWithReturn(
	viewType TimelineViewType,
	pr *domain.PaymentRequest,
	order *domain.Order,
	job *domain.DeliveryJob,
	returnJob *domain.DeliveryJob,
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
		steps, currentStep = appendDeliveryStepsWithReturn(steps, currentStep, viewType, order, pr, job, returnJob)
	} else {
		steps, currentStep = appendSimpleSteps(steps, currentStep, viewType, order)
	}

	// Determine phase
	phase := "delivery"
	if pr.Status.IsReturnFlow() || (pr.Status == domain.PRStatusRefunded && returnJob != nil) {
		phase = "return"
	}

	resp := TimelineResponse{
		Steps:        steps,
		CurrentStep:  currentStep,
		ItemName:     pr.ItemName,
		TotalAmount:  order.TotalAmount,
		DeliveryType: string(pr.DeliveryType),
		Phase:        phase,
	}

	// Set codes based on phase
	if phase == "return" && returnJob != nil && returnJob.RiderID != nil {
		// In return phase: roles are reversed
		// Buyer is at pickup (gets pickup code), Seller is at dropoff (gets dropoff code)
		if viewType == TimelineViewBuyer {
			resp.PickupCode = returnJob.PickupCode
		} else {
			resp.DropoffCode = returnJob.DropoffCode
		}
	} else if job != nil && job.RiderID != nil {
		// Normal delivery phase
		if viewType == TimelineViewSeller {
			resp.PickupCode = job.PickupCode
		} else {
			resp.DropoffCode = job.DropoffCode
		}
	}

	return resp
}

func appendDeliveryStepsWithReturn(
	steps []TimelineStep,
	currentStep string,
	viewType TimelineViewType,
	order *domain.Order,
	pr *domain.PaymentRequest,
	job *domain.DeliveryJob,
	returnJob *domain.DeliveryJob,
) ([]TimelineStep, string) {
	// Check if we're in rebook flow (buyer unavailable)
	isRebookFlow := pr.Status == domain.PRStatusPendingRebook || pr.Status == domain.PRStatusRebookExpired ||
		(job != nil && (job.Status == domain.DJStatusBuyerUnavailable || job.Status == domain.DJStatusForcedReturn || job.Status == domain.DJStatusForcedReturnDone))

	// Check if we're in dispute/return flow
	// Also show return steps if refunded with a return job (completed return flow)
	wasReturnCompleted := pr.Status == domain.PRStatusRefunded && returnJob != nil && !returnJob.IsForcedReturn

	// If in rebook flow, use specialized delivery steps (no "delivered" step)
	if isRebookFlow {
		steps, currentStep = appendDeliveryStepsForUnavailability(steps, currentStep, viewType, order, job)
		steps, currentStep = appendRebookFlowSteps(steps, currentStep, viewType, pr, job)
		return steps, currentStep
	}

	// Normal flow - add regular delivery steps
	steps, currentStep = appendDeliverySteps(steps, currentStep, viewType, order, job)

	if !pr.Status.IsReturnFlow() && pr.Status != domain.PRStatusDisputed && !wasReturnCompleted {
		return steps, currentStep
	}

	// Add dispute step if disputed
	if order.Status == domain.OrderStatusDisputed || pr.Status.IsReturnFlow() || wasReturnCompleted {
		disputeDesc := "You opened a dispute"
		if viewType == TimelineViewSeller {
			disputeDesc = "Buyer opened a dispute"
		}
		disputeStep := TimelineStep{
			Step:        "disputed",
			Title:       "Dispute Opened",
			Description: disputeDesc,
			Status:      "completed",
		}
		steps = append(steps, disputeStep)
	}

	// Add return flow steps if return is required or was completed
	if pr.Status.IsReturnFlow() || pr.Status == domain.PRStatusReturnRequired || wasReturnCompleted {
		// Return awaiting confirmation step (buyer must confirm they're ready)
		awaitingConfirmDesc := "Confirm you're ready for return pickup"
		if viewType == TimelineViewSeller {
			awaitingConfirmDesc = "Waiting for buyer to confirm they're ready for pickup"
		}
		awaitingConfirmStep := TimelineStep{
			Step:        "return_awaiting_confirmation",
			Title:       "Awaiting Confirmation",
			Description: awaitingConfirmDesc,
			Status:      "pending",
		}

		// Return searching step
		returnSearchDesc := "Searching for rider to return item"
		if viewType == TimelineViewSeller {
			returnSearchDesc = "Searching for rider to return item to you"
		}
		returnSearchStep := TimelineStep{
			Step:        "return_searching",
			Title:       "Return Pickup Scheduled",
			Description: returnSearchDesc,
			Status:      "pending",
		}

		// Return rider assigned step
		returnRiderDesc := "Rider is on the way to pick up item for return"
		if viewType == TimelineViewSeller {
			returnRiderDesc = "Rider is picking up item from buyer"
		}
		returnRiderStep := TimelineStep{
			Step:        "return_rider_assigned",
			Title:       "Return Rider Assigned",
			Description: returnRiderDesc,
			Status:      "pending",
		}

		// Return picked up step
		returnPickedUpDesc := "Item picked up from you for return"
		if viewType == TimelineViewSeller {
			returnPickedUpDesc = "Item picked up from buyer"
		}
		returnPickedUpStep := TimelineStep{
			Step:        "return_picked_up",
			Title:       "Return Item Picked Up",
			Description: returnPickedUpDesc,
			Status:      "pending",
		}

		// Return in transit step
		returnTransitStep := TimelineStep{
			Step:        "return_in_transit",
			Title:       "Return In Transit",
			Description: "Item is being returned",
			Status:      "pending",
		}

		// Return delivered step
		returnDeliveredDesc := "Item returned to seller"
		if viewType == TimelineViewSeller {
			returnDeliveredDesc = "Item returned to you. Please confirm condition."
		}
		returnDeliveredStep := TimelineStep{
			Step:        "return_delivered",
			Title:       "Return Delivered",
			Description: returnDeliveredDesc,
			Status:      "pending",
		}

		// Return confirmed / refund step
		returnConfirmedDesc := "Return confirmed. Refund processed."
		if viewType == TimelineViewSeller {
			returnConfirmedDesc = "Return confirmed. Buyer refunded."
		}
		returnConfirmedStep := TimelineStep{
			Step:        "return_confirmed",
			Title:       "Return Confirmed",
			Description: returnConfirmedDesc,
			Status:      "pending",
		}

		// Update return steps based on return job status
		if returnJob != nil {
			switch returnJob.Status {
			case domain.DJStatusCreated:
				// Job created but buyer hasn't confirmed ready yet
				awaitingConfirmStep.Status = "current"
				currentStep = "return_awaiting_confirmation"
				if !returnJob.CreatedAt.IsZero() {
					awaitingConfirmStep.Timestamp = returnJob.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
				}

			case domain.DJStatusSearching:
				awaitingConfirmStep.Status = "completed"
				returnSearchStep.Status = "current"
				currentStep = "return_searching"
				if returnJob.SearchStartedAt != nil {
					returnSearchStep.Timestamp = returnJob.SearchStartedAt.Format("2006-01-02T15:04:05Z07:00")
				}

			case domain.DJStatusAccepted, domain.DJStatusEnRoutePickup:
				awaitingConfirmStep.Status = "completed"
				returnSearchStep.Status = "completed"
				returnRiderStep.Status = "current"
				currentStep = "return_rider_assigned"
				if returnJob.AcceptedAt != nil {
					returnRiderStep.Timestamp = returnJob.AcceptedAt.Format("2006-01-02T15:04:05Z07:00")
				}

			case domain.DJStatusPickedUp:
				awaitingConfirmStep.Status = "completed"
				returnSearchStep.Status = "completed"
				returnRiderStep.Status = "completed"
				returnPickedUpStep.Status = "current"
				currentStep = "return_picked_up"
				if returnJob.PickedUpAt != nil {
					returnPickedUpStep.Timestamp = returnJob.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
				}

			case domain.DJStatusInTransit:
				awaitingConfirmStep.Status = "completed"
				returnSearchStep.Status = "completed"
				returnRiderStep.Status = "completed"
				returnPickedUpStep.Status = "completed"
				returnTransitStep.Status = "current"
				currentStep = "return_in_transit"

			case domain.DJStatusDelivered, domain.DJStatusReturnDelivered:
				awaitingConfirmStep.Status = "completed"
				returnSearchStep.Status = "completed"
				returnRiderStep.Status = "completed"
				returnPickedUpStep.Status = "completed"
				returnTransitStep.Status = "completed"
				returnDeliveredStep.Status = "current"
				currentStep = "return_delivered"
				if returnJob.DeliveredAt != nil {
					returnDeliveredStep.Timestamp = returnJob.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
				}

			case domain.DJStatusReturnConfirmed, domain.DJStatusReturnRefunded:
				awaitingConfirmStep.Status = "completed"
				returnSearchStep.Status = "completed"
				returnRiderStep.Status = "completed"
				returnPickedUpStep.Status = "completed"
				returnTransitStep.Status = "completed"
				returnDeliveredStep.Status = "completed"
				returnConfirmedStep.Status = "completed"
				currentStep = "return_confirmed"
				if returnJob.ReturnConfirmedAt != nil {
					returnConfirmedStep.Timestamp = returnJob.ReturnConfirmedAt.Format("2006-01-02T15:04:05Z07:00")
				}

			case domain.DJStatusReturnDamageDisputed:
				awaitingConfirmStep.Status = "completed"
				returnSearchStep.Status = "completed"
				returnRiderStep.Status = "completed"
				returnPickedUpStep.Status = "completed"
				returnTransitStep.Status = "completed"
				returnDeliveredStep.Status = "completed"
				// Override confirmed step for damage dispute
				returnConfirmedStep.Title = "Damage Claim"
				returnConfirmedStep.Description = "Seller reported damage. Admin reviewing."
				returnConfirmedStep.Status = "current"
				currentStep = "return_confirmed"
			}
		} else {
			// Return required but job not created yet - buyer needs to confirm ready
			awaitingConfirmStep.Status = "current"
			currentStep = "return_awaiting_confirmation"
		}

		steps = append(steps, awaitingConfirmStep, returnSearchStep, returnRiderStep, returnPickedUpStep,
			returnTransitStep, returnDeliveredStep, returnConfirmedStep)
	}

	return steps, currentStep
}

// appendDeliveryStepsForUnavailability adds delivery steps up to in_transit (no delivered step)
// Used when buyer is unavailable - delivery hasn't completed
func appendDeliveryStepsForUnavailability(
	steps []TimelineStep,
	currentStep string,
	viewType TimelineViewType,
	_ *domain.Order,
	job *domain.DeliveryJob,
) ([]TimelineStep, string) {
	awaitingStep := TimelineStep{
		Step:        "awaiting_rider",
		Title:       "Awaiting Rider Assignment",
		Description: "Searching for a delivery rider",
		Status:      "completed",
	}
	riderStep := TimelineStep{
		Step:        "rider_assigned",
		Title:       "Rider Assigned",
		Description: "A rider has been assigned to your delivery",
		Status:      "completed",
	}
	pickedUpStep := TimelineStep{
		Step:        "picked_up",
		Title:       "Item Picked Up",
		Description: "Rider has collected the item from seller",
		Status:      "completed",
	}
	inTransitStep := TimelineStep{
		Step:        "in_transit",
		Title:       "In Transit",
		Description: "Item was on the way to you",
		Status:      "completed",
	}

	if viewType == TimelineViewSeller {
		awaitingStep.Title = "Ready for Pickup"
		awaitingStep.Description = "Rider search completed"
		riderStep.Description = "A rider collected the item"
		pickedUpStep.Description = "Rider collected the item"
		inTransitStep.Description = "Item was on the way to buyer"
	}

	// Add timestamps if available
	if job != nil {
		if job.AcceptedAt != nil {
			riderStep.Timestamp = job.AcceptedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		if job.PickedUpAt != nil {
			pickedUpStep.Timestamp = job.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
		}
	}

	steps = append(steps, awaitingStep, riderStep, pickedUpStep, inTransitStep)
	return steps, currentStep
}

// appendRebookFlowSteps adds buyer unavailable -> rebook/return flow steps
func appendRebookFlowSteps(
	steps []TimelineStep,
	currentStep string,
	viewType TimelineViewType,
	pr *domain.PaymentRequest,
	job *domain.DeliveryJob,
) ([]TimelineStep, string) {
	// Buyer unavailable step
	unavailableDesc := "You were unavailable at the delivery location"
	if viewType == TimelineViewSeller {
		unavailableDesc = "Buyer was unavailable at delivery location"
	}
	unavailableStep := TimelineStep{
		Step:        "buyer_unavailable",
		Title:       "Buyer Unavailable",
		Description: unavailableDesc,
		Status:      "pending",
	}

	// Pending rebook step (45-min window)
	rebookDesc := "Rebook delivery within 45 minutes to continue"
	if viewType == TimelineViewSeller {
		rebookDesc = "Waiting for buyer to rebook or admin contact"
	}
	rebookStep := TimelineStep{
		Step:        "pending_rebook",
		Title:       "Rebook Window",
		Description: rebookDesc,
		Status:      "pending",
	}

	// Forced return step (if rebook window expired)
	forcedReturnDesc := "Item is being returned to seller"
	if viewType == TimelineViewSeller {
		forcedReturnDesc = "Item is being returned to you"
	}
	forcedReturnStep := TimelineStep{
		Step:        "forced_return",
		Title:       "Returning to Seller",
		Description: forcedReturnDesc,
		Status:      "pending",
	}

	// Refund step (final)
	refundDesc := "Partial refund processed (minus 5% unavailability fee)"
	if viewType == TimelineViewSeller {
		refundDesc = "Buyer received partial refund"
	}
	refundStep := TimelineStep{
		Step:        "partial_refund",
		Title:       "Partial Refund",
		Description: refundDesc,
		Status:      "pending",
	}

	// Determine current status based on job and PR status
	if job != nil {
		switch job.Status {
		case domain.DJStatusBuyerUnavailable:
			// Rider still has item, 45-min window active
			unavailableStep.Status = "completed"
			if job.UnavailableMarkedAt != nil {
				unavailableStep.Timestamp = job.UnavailableMarkedAt.Format("2006-01-02T15:04:05Z07:00")
			}
			rebookStep.Status = "current"
			currentStep = "pending_rebook"

		case domain.DJStatusForcedReturn:
			// 45-min expired, rider returning to seller
			unavailableStep.Status = "completed"
			rebookStep.Status = "completed"
			rebookStep.Title = "Rebook Expired"
			rebookStep.Description = "45-minute window expired"
			forcedReturnStep.Status = "current"
			currentStep = "forced_return"

		case domain.DJStatusForcedReturnDone:
			// Rider returned item to seller
			unavailableStep.Status = "completed"
			rebookStep.Status = "completed"
			rebookStep.Title = "Rebook Expired"
			forcedReturnStep.Status = "completed"
			forcedReturnStep.Title = "Returned to Seller"
			forcedReturnStep.Description = "Item has been returned to seller"
			if job.DeliveredAt != nil {
				forcedReturnStep.Timestamp = job.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
			}
			refundStep.Status = "current"
			currentStep = "partial_refund"
		}
	}

	// Override based on PR status for final states
	if pr.Status == domain.PRStatusRebookExpired {
		unavailableStep.Status = "completed"
		rebookStep.Status = "completed"
		rebookStep.Title = "Rebook Expired"
	}
	if pr.Status == domain.PRStatusRefunded {
		refundStep.Status = "completed"
		currentStep = "partial_refund"
	}

	steps = append(steps, unavailableStep, rebookStep, forcedReturnStep, refundStep)
	return steps, currentStep
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
		// Note: DJStatusBuyerUnavailable, DJStatusForcedReturn, DJStatusForcedReturnDone
		// are handled separately in appendDeliveryStepsForUnavailability + appendRebookFlowSteps
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
	Description *string        `json:"package_description,omitempty"`
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
