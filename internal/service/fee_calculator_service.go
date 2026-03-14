package service

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"time"

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

	CalculatePlatformDeliveryFee(deliveryFee float64) float64

	GetZoneByLocation(ctx context.Context, lat, lng float64) (*domain.DeliveryZone, error)
}

type DeliveryFeeEstimate struct {
	ZoneID          uuid.UUID `json:"zone_id"`
	ZoneName        string    `json:"zone_name"`
	DistanceKM      float64   `json:"distance_km"`
	DurationMinutes int       `json:"duration_minutes"`
	BaseFee         float64   `json:"base_fee"`
	PerKMFee        float64   `json:"per_km_fee"`
	TotalFee        float64   `json:"total_fee"`
	IsEstimate      bool      `json:"is_estimate"`
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
	zoneRepo     repository.DeliveryZoneRepository
	settingsRepo repository.PlatformSettingsRepository
	mapsClient   maps.MapsClient
	config       *config.Config
	logger       *zap.Logger
	pricingCache   *domain.DeliveryPricing
	feesCache      *domain.PlatformFees
	cacheExpiry    time.Time
	cacheMutex     sync.RWMutex
	cacheDuration  time.Duration
}

const defaultCacheDuration = 5 * time.Minute

func NewFeeCalculatorService(
	zoneRepo repository.DeliveryZoneRepository,
	settingsRepo repository.PlatformSettingsRepository,
	mapsClient maps.MapsClient,
	cfg *config.Config,
	logger *zap.Logger,
) FeeCalculatorService {
	return &FeeCalculatorServiceImpl{
		zoneRepo:      zoneRepo,
		settingsRepo:  settingsRepo,
		mapsClient:    mapsClient,
		config:        cfg,
		logger:        logger,
		cacheDuration: defaultCacheDuration,
	}
}
func (s *FeeCalculatorServiceImpl) CalculateDeliveryFee(ctx context.Context, pickupLat, pickupLng, dropoffLat, dropoffLng float64) (*DeliveryFeeEstimate, error) {
	var distanceKM float64
	var durationMinutes int
	if s.mapsClient != nil {
		etaResult, err := s.mapsClient.GetETA(ctx, pickupLat, pickupLng, dropoffLat, dropoffLng)
		if err != nil {
			s.logger.Warn("failed to get road distance from Google Maps, using Haversine fallback",
				zap.Error(err),
			)
			distanceKM = s.calculateDistanceKM(pickupLat, pickupLng, dropoffLat, dropoffLng)
			durationMinutes = int(distanceKM * 2)
		} else {
			distanceKM = etaResult.DistanceKM
			durationMinutes = etaResult.DurationMinutes
		}
	} else {
		distanceKM = s.calculateDistanceKM(pickupLat, pickupLng, dropoffLat, dropoffLng)
		durationMinutes = int(distanceKM * 2)
	}

	baseFee := s.getBaseDeliveryFee()  // Default: 500 NGN
	perKMFee := s.getPerKMFee()        // Default: 100 NGN per km

	totalFee := baseFee + (distanceKM * perKMFee)
	totalFee = math.Round(totalFee*100) / 100

	return &DeliveryFeeEstimate{
		DistanceKM:      distanceKM,
		DurationMinutes: durationMinutes,
		BaseFee:         baseFee,
		PerKMFee:        perKMFee,
		TotalFee:        totalFee,
		IsEstimate:      true,
	}, nil
}
func (s *FeeCalculatorServiceImpl) CalculateDeliveryFeeWithVehicle(ctx context.Context, pickupLat, pickupLng, dropoffLat, dropoffLng float64, vehicleType domain.VehicleType) (*DeliveryFeeEstimate, error) {
	estimate, err := s.CalculateDeliveryFee(ctx, pickupLat, pickupLng, dropoffLat, dropoffLng)
	if err != nil {
		return nil, err
	}
	estimate.TotalFee = s.ApplyVehicleTypeMultiplier(estimate.TotalFee, vehicleType)

	return estimate, nil
}
func (s *FeeCalculatorServiceImpl) ApplyVehicleTypeMultiplier(baseFee float64, vehicleType domain.VehicleType) float64 {
	multiplier := s.getVehicleTypeMultiplier(vehicleType)
	return math.Round(baseFee*multiplier*100) / 100
}
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
func (s *FeeCalculatorServiceImpl) CalculateBuyerFee(itemPrice float64) float64 {
	rate := s.getBuyerFeeRate()
	return math.Round(itemPrice*rate*100) / 100
}
func (s *FeeCalculatorServiceImpl) CalculateSellerFee(itemPrice float64) float64 {
	rate := s.getSellerFeeRate()
	return math.Round(itemPrice*rate*100) / 100
}
func (s *FeeCalculatorServiceImpl) CalculateTotalForBuyer(itemPrice float64, deliveryFee float64) float64 {
	buyerFee := s.CalculateBuyerFee(itemPrice)
	return itemPrice + deliveryFee + buyerFee
}
func (s *FeeCalculatorServiceImpl) CalculateSettlement(itemPrice, actualDeliveryFee float64) *SettlementBreakdown {
	buyerServiceFee := s.CalculateBuyerFee(itemPrice)
	sellerPlatformFee := s.CalculateSellerFee(itemPrice)
	platformDeliveryFee := s.CalculatePlatformDeliveryFee(actualDeliveryFee)

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
func (s *FeeCalculatorServiceImpl) CalculateRiderPayout(deliveryFee float64) float64 {
	rate := s.getRiderPayoutRate()
	return math.Round(deliveryFee*rate*100) / 100
}
func (s *FeeCalculatorServiceImpl) GetZoneByLocation(ctx context.Context, lat, lng float64) (*domain.DeliveryZone, error) {
	return s.zoneRepo.GetByLocation(ctx, lat, lng)
}
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
	return math.Round(distance*100) / 100
}
func (s *FeeCalculatorServiceImpl) CalculatePlatformDeliveryFee(deliveryFee float64) float64 {
	rate := 1.0 - s.getRiderPayoutRate()
	return math.Round(deliveryFee*rate*100) / 100
}
func (s *FeeCalculatorServiceImpl) getBuyerFeeRate() float64 {
	fees := s.getPlatformFees()
	if fees != nil && fees.BuyerFeeRate > 0 {
		return fees.BuyerFeeRate
	}
	return 0.02 // fallback
}

func (s *FeeCalculatorServiceImpl) getSellerFeeRate() float64 {
	fees := s.getPlatformFees()
	if fees != nil && fees.SellerFeeRate > 0 {
		return fees.SellerFeeRate
	}
	return 0.03 // fallback
}

func (s *FeeCalculatorServiceImpl) getRiderPayoutRate() float64 {
	fees := s.getPlatformFees()
	if fees != nil && fees.RiderPayoutRate > 0 {
		return fees.RiderPayoutRate
	}
	return 0.85 // fallback
}

func (s *FeeCalculatorServiceImpl) getBaseDeliveryFee() float64 {
	pricing := s.getDeliveryPricing()
	if pricing != nil && pricing.BaseFee > 0 {
		return pricing.BaseFee
	}
	return 500.0 // fallback
}

func (s *FeeCalculatorServiceImpl) getPerKMFee() float64 {
	pricing := s.getDeliveryPricing()
	if pricing != nil && pricing.PerKMFee > 0 {
		return pricing.PerKMFee
	}
	return 100.0 // fallback
}

func (s *FeeCalculatorServiceImpl) getDeliveryPricing() *domain.DeliveryPricing {
	s.cacheMutex.RLock()
	if s.pricingCache != nil && time.Now().Before(s.cacheExpiry) {
		pricing := s.pricingCache
		s.cacheMutex.RUnlock()
		return pricing
	}
	s.cacheMutex.RUnlock()
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	if s.pricingCache != nil && time.Now().Before(s.cacheExpiry) {
		return s.pricingCache
	}

	if s.settingsRepo == nil {
		return nil
	}

	settings, err := s.settingsRepo.Get(context.Background(), "delivery_pricing")
	if err != nil {
		s.logger.Warn("failed to get delivery pricing from database, using fallback", zap.Error(err))
		return nil
	}

	var pricing domain.DeliveryPricing
	if err := json.Unmarshal(settings.SettingValue, &pricing); err != nil {
		s.logger.Warn("failed to unmarshal delivery pricing, using fallback", zap.Error(err))
		return nil
	}

	s.pricingCache = &pricing
	s.cacheExpiry = time.Now().Add(s.cacheDuration)
	s.logger.Debug("loaded delivery pricing from database", zap.Float64("base_fee", pricing.BaseFee), zap.Float64("per_km_fee", pricing.PerKMFee))

	return &pricing
}

func (s *FeeCalculatorServiceImpl) getPlatformFees() *domain.PlatformFees {
	s.cacheMutex.RLock()
	if s.feesCache != nil && time.Now().Before(s.cacheExpiry) {
		fees := s.feesCache
		s.cacheMutex.RUnlock()
		return fees
	}
	s.cacheMutex.RUnlock()
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	if s.feesCache != nil && time.Now().Before(s.cacheExpiry) {
		return s.feesCache
	}

	if s.settingsRepo == nil {
		return nil
	}

	settings, err := s.settingsRepo.Get(context.Background(), "platform_fees")
	if err != nil {
		s.logger.Warn("failed to get platform fees from database, using fallback", zap.Error(err))
		return nil
	}

	var fees domain.PlatformFees
	if err := json.Unmarshal(settings.SettingValue, &fees); err != nil {
		s.logger.Warn("failed to unmarshal platform fees, using fallback", zap.Error(err))
		return nil
	}

	s.feesCache = &fees
	s.cacheExpiry = time.Now().Add(s.cacheDuration)
	s.logger.Debug("loaded platform fees from database",
		zap.Float64("buyer_fee_rate", fees.BuyerFeeRate),
		zap.Float64("seller_fee_rate", fees.SellerFeeRate),
		zap.Float64("rider_payout_rate", fees.RiderPayoutRate),
	)

	return &fees
}
var _ FeeCalculatorService = (*FeeCalculatorServiceImpl)(nil)
