package handler

import (
	"log"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

// ListSchools handles GET /dictionaries/schools
// Supports two mutually exclusive modes:
//   - Keyword search: ?keyword=xxx  → fuzzy match on school_name, limit 30
//   - Location filter: ?province=&city=&district= → exact match, return all
func (s *Server) ListSchools(ctx echo.Context, params api.ListSchoolsParams) error {
	schools, err := s.repo.School.List(ctx.Request().Context(), repository.SchoolListParams{
		Keyword:  params.Keyword,
		Province: params.Province,
		City:     params.City,
		District: params.District,
	})
	if err != nil {
		log.Printf("ListSchools error: %v", err)
		return InternalError(ctx, "获取学校列表失败")
	}

	var schoolVOs []api.SchoolVO
	for _, school := range schools {
		schoolVOs = append(schoolVOs, *school.ToVO())
	}
	if schoolVOs == nil {
		schoolVOs = []api.SchoolVO{}
	}

	return Success(ctx, schoolVOs)
}

// GetSchoolProvinces handles GET /dictionaries/schools/provinces
func (s *Server) GetSchoolProvinces(ctx echo.Context) error {
	provinces, err := s.repo.School.ListProvinces(ctx.Request().Context())
	if err != nil {
		log.Printf("GetSchoolProvinces error: %v", err)
		return InternalError(ctx, "获取省份列表失败")
	}
	if provinces == nil {
		provinces = []string{}
	}
	return Success(ctx, provinces)
}

// GetSchoolCities handles GET /dictionaries/schools/cities?province=xxx
func (s *Server) GetSchoolCities(ctx echo.Context) error {
	province := ctx.QueryParam("province")
	if province == "" {
		return Success(ctx, []string{})
	}

	cities, err := s.repo.School.ListCities(ctx.Request().Context(), province)
	if err != nil {
		log.Printf("GetSchoolCities error: %v", err)
		return InternalError(ctx, "获取城市列表失败")
	}
	if cities == nil {
		cities = []string{}
	}
	return Success(ctx, cities)
}

// GetSchoolDistricts handles GET /dictionaries/schools/districts?province=xxx&city=xxx
func (s *Server) GetSchoolDistricts(ctx echo.Context) error {
	province := ctx.QueryParam("province")
	city := ctx.QueryParam("city")
	if province == "" || city == "" {
		return Success(ctx, []string{})
	}

	districts, err := s.repo.School.ListDistricts(ctx.Request().Context(), province, city)
	if err != nil {
		log.Printf("GetSchoolDistricts error: %v", err)
		return InternalError(ctx, "获取区县列表失败")
	}
	if districts == nil {
		districts = []string{}
	}
	return Success(ctx, districts)
}

// ListMajors handles GET /dictionaries/majors
func (s *Server) ListMajors(ctx echo.Context, params api.ListMajorsParams) error {
	classes, err := s.repo.Major.ListWithMajors(ctx.Request().Context(), params)
	if err != nil {
		log.Printf("ListMajors error: %v", err)
		return InternalError(ctx, "获取专业列表失败")
	}

	// Convert to VOs
	var classVOs []api.MajorClassVO
	for _, mc := range classes {
		classVOs = append(classVOs, *mc.ToVO())
	}

	return Success(ctx, classVOs)
}
