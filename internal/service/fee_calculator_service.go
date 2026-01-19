package service

import (
	"context"
	"math"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/config"
	"kipa/internal/domain"
	"kipa/internal/integration/maps"
	"kipa/internal/repository"
)

type FeeCalculatorService interface {
	CalculateDeliveryFee(ctx context.Context, pickupLat, pickupLng, dropoffLat, dropoffLng float64) (*DeliveryFeeEstimate, error)

	CalculateDeliveryFeeWithVehicle(ctx context.Context, pickupLat, pickupLng, dropoffLat, dropoffLng float64, vehicleType domain.VehicleType) (*DeliveryFeeEstimate, error)

	ApplyVehicleTypeMultiplier(baseFee float64, vehicleType domain.VehicleType) float64

	CalculateBuyerFee(itemPrice float64) float64

	CalculateSellerFee(itemPrice float64) float64

	CalculateTotalForBuyer(itemPrice float64, deliveryFee float64) float64

	CalculateSettlement(itemPrice, actualDeliveryFee float64) *SettlementBreakdown

	CalculateRiderPayout(deliveryFee float64) float64

	GetZoneByLocation(ctx context.Context, lat, lng float64) (*domain.DeliveryZone, error)
}

type DeliveryFeeEstimate struct {
	ZoneID      uuid.UUID `json:"zone_id"`
	ZoneName    string    `json:"zone_name"`
	DistanceKM  float64   `json:"distance_km"`
	BaseFee     float64   `json:"base_fee"`
	PerKMFee    float64   `json:"per_km_fee"`
	TotalFee    float64   `json:"total_fee"`
	IsEstimate  bool      `json:"is_estimate"` 
}

type SettlementBreakdown struct {
	ItemPrice           float64 `json:"item_price"`
	DeliveryFee         float64 `json:"delivery_fee"`
	BuyerServiceFee     float64 `json:"buyer_service_fee"`
	SellerPlatformFee   float64 `json:"seller_platform_fee"`
	PlatformDeliveryFee float64 `json:"platform_delivery_fee"`
	SellerPayout        float64 `json:"seller_payout"`
	RiderPayout         float64 `json:"rider_payout"`
	PlatformRevenue     float64 `json:"platform_revenue"`
}

type FeeCalculatorServiceImpl struct {
	zoneRepo   repository.DeliveryZoneRepository
	mapsClient maps.MapsClient
	config     *config.Config
	logger     *zap.Logger
}

func NewFeeCalculatorService(
	zoneRepo repository.DeliveryZoneRepository,
	mapsClient maps.MapsClient,
	cfg *config.Config,
	logger *zap.Logger,
) FeeCalculatorService {
	return &FeeCalculatorServiceImpl{
		zoneRepo:   zoneRepo,
		mapsClient: mapsClient,
		config:     cfg,
		logger:     logger,
	}
}

// CalculateDeliveryFee calculates the estimated delivery fee based on road distance
// Uses Google Distance Matrix API for accurate road distance
func (s *FeeCalculatorServiceImpl) CalculateDeliveryFee(ctx context.Context, pickupLat, pickupLng, dropoffLat, dropoffLng float64) (*DeliveryFeeEstimate, error) {
	var distanceKM float64
	
	// Google Maps API for accurate road distance
	if s.mapsClient != nil {
		etaResult, err := s.mapsClient.GetETA(ctx, pickupLat, pickupLng, dropoffLat, dropoffLng)
		if err != nil {
			s.logger.Warn("failed to get road distance from Google Maps, using Haversine fallback",
				zap.Error(err),
			)
			// Fallback to Haversine (straight-line) calculation
			distanceKM = s.calculateDistanceKM(pickupLat, pickupLng, dropoffLat, dropoffLng)
		} else {
			distanceKM = etaResult.DistanceKM
		}
	} else {
		// No maps client, use Haversine
		distanceKM = s.calculateDistanceKM(pickupLat, pickupLng, dropoffLat, dropoffLng)
	}

	
	baseFee := s.getBaseDeliveryFee()  // Default: 500 NGN
	perKMFee := s.getPerKMFee()        // Default: 100 NGN per km

	
	totalFee := baseFee + (distanceKM * perKMFee)
	totalFee = math.Round(totalFee*100) / 100

	return &DeliveryFeeEstimate{
		DistanceKM: distanceKM,
		BaseFee:    baseFee,
		PerKMFee:   perKMFee,
		TotalFee:   totalFee,
		IsEstimate: true,
	}, nil
}

// CalculateDeliveryFeeWithVehicle calculates delivery fee with vehicle type multiplier applied
func (s *FeeCalculatorServiceImpl) CalculateDeliveryFeeWithVehicle(ctx context.Context, pickupLat, pickupLng, dropoffLat, dropoffLng float64, vehicleType domain.VehicleType) (*DeliveryFeeEstimate, error) {
	estimate, err := s.CalculateDeliveryFee(ctx, pickupLat, pickupLng, dropoffLat, dropoffLng)
	if err != nil {
		return nil, err
	}

	// Apply vehicle type multiplier
	estimate.TotalFee = s.ApplyVehicleTypeMultiplier(estimate.TotalFee, vehicleType)

	return estimate, nil
}

// ApplyVehicleTypeMultiplier applies the vehicle type multiplier to a base fee
// Cars have a 50% higher fee than motorcycles
func (s *FeeCalculatorServiceImpl) ApplyVehicleTypeMultiplier(baseFee float64, vehicleType domain.VehicleType) float64 {
	multiplier := s.getVehicleTypeMultiplier(vehicleType)
	return math.Round(baseFee*multiplier*100) / 100
}

// getVehicleTypeMultiplier returns the fee multiplier for a given vehicle type
func (s *FeeCalculatorServiceImpl) getVehicleTypeMultiplier(vehicleType domain.VehicleType) float64 {
	switch vehicleType {
	case domain.VehicleTypeCar:
		return 1.5 
	case domain.VehicleTypeMotorcycle:
		return 1.0 
	default:
		return 1.0
	}
}

// CalculateBuyerFee calculates the buyer's service fee (2% of item price)
func (s *FeeCalculatorServiceImpl) CalculateBuyerFee(itemPrice float64) float64 {
	rate := s.getBuyerFeeRate()
	return math.Round(itemPrice*rate*100) / 100
}

// CalculateSellerFee calculates the seller's platform fee (3% of item price)
func (s *FeeCalculatorServiceImpl) CalculateSellerFee(itemPrice float64) float64 {
	rate := s.getSellerFeeRate()
	return math.Round(itemPrice*rate*100) / 100
}

// CalculateTotalForBuyer calculates the total amount buyer needs to pay
func (s *FeeCalculatorServiceImpl) CalculateTotalForBuyer(itemPrice float64, deliveryFee float64) float64 {
	buyerFee := s.CalculateBuyerFee(itemPrice)
	return itemPrice + deliveryFee + buyerFee
}

// CalculateSettlement calculates the final payout amounts for seller, rider, and platform
func (s *FeeCalculatorServiceImpl) CalculateSettlement(itemPrice, actualDeliveryFee float64) *SettlementBreakdown {
	buyerServiceFee := s.CalculateBuyerFee(itemPrice)
	sellerPlatformFee := s.CalculateSellerFee(itemPrice)
	platformDeliveryFee := s.calculatePlatformDeliveryFee(actualDeliveryFee)

	sellerPayout := itemPrice - sellerPlatformFee
	riderPayout := actualDeliveryFee - platformDeliveryFee
	platformRevenue := buyerServiceFee + sellerPlatformFee + platformDeliveryFee

	return &SettlementBreakdown{
		ItemPrice:           itemPrice,
		DeliveryFee:         actualDeliveryFee,
		BuyerServiceFee:     math.Round(buyerServiceFee*100) / 100,
		SellerPlatformFee:   math.Round(sellerPlatformFee*100) / 100,
		PlatformDeliveryFee: math.Round(platformDeliveryFee*100) / 100,
		SellerPayout:        math.Round(sellerPayout*100) / 100,
		RiderPayout:         math.Round(riderPayout*100) / 100,
		PlatformRevenue:     math.Round(platformRevenue*100) / 100,
	}
}

// CalculateRiderPayout calculates how much rider receives from delivery fee (85%)
func (s *FeeCalculatorServiceImpl) CalculateRiderPayout(deliveryFee float64) float64 {
	rate := s.getRiderPayoutRate()
	return math.Round(deliveryFee*rate*100) / 100
}

// GetZoneByLocation gets the delivery zone for a given location
func (s *FeeCalculatorServiceImpl) GetZoneByLocation(ctx context.Context, lat, lng float64) (*domain.DeliveryZone, error) {
	return s.zoneRepo.GetByLocation(ctx, lat, lng)
}

// calculateDistanceKM calculates the distance between two points using the Haversine formula
func (s *FeeCalculatorServiceImpl) calculateDistanceKM(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKM = 6371.0

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLng/2)*math.Sin(deltaLng/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	distance := earthRadiusKM * c

	// Round to 2 decimal places
	return math.Round(distance*100) / 100
}

// calculatePlatformDeliveryFee calculates platform's cut of delivery fee (15%)
func (s *FeeCalculatorServiceImpl) calculatePlatformDeliveryFee(deliveryFee float64) float64 {
	rate := 1.0 - s.getRiderPayoutRate()
	return math.Round(deliveryFee*rate*100) / 100
}

// Fee rates - these could be moved to config if needed
func (s *FeeCalculatorServiceImpl) getBuyerFeeRate() float64 {
	// 2% buyer service fee
	return 0.02
}

func (s *FeeCalculatorServiceImpl) getSellerFeeRate() float64 {
	// 3% seller platform fee
	return 0.03
}

func (s *FeeCalculatorServiceImpl) getRiderPayoutRate() float64 {
	// Rider keeps 85% of delivery fee
	return 0.85
}

// Delivery fee rate methods
func (s *FeeCalculatorServiceImpl) getBaseDeliveryFee() float64 {
	// Base delivery fee in NGN
	return 500.0
}

func (s *FeeCalculatorServiceImpl) getPerKMFee() float64 {
	// Per kilometer fee in NGN
	return 100.0
}

// Ensure FeeCalculatorServiceImpl implements FeeCalculatorService
var _ FeeCalculatorService = (*FeeCalculatorServiceImpl)(nil)
