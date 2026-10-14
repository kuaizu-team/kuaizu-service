package handler

import (
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
	"net/http"
	"strconv"
)

func cooperationPageParams(ctx echo.Context) (int, int, error) {
	page, size := 1, 20
	for _, field := range []struct {
		name   string
		target *int
	}{{"page", &page}, {"size", &size}} {
		if value := ctx.QueryParam(field.name); value != "" {
			number, err := strconv.Atoi(value)
			if err != nil || number < 1 || (field.name == "page" && number > 1000000) {
				return 0, 0, service.ErrBadRequest("分页参数无效")
			}
			*field.target = number
		}
	}
	if size > 50 {
		size = 50
	}
	return page, size, nil
}
func cooperationResourceID(ctx echo.Context) (int64, error) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, service.ErrBadRequest("合作资源ID无效")
	}
	return id, nil
}
func (s *AdminServer) ListCommercialOrders(ctx echo.Context) error {
	page, size, err := cooperationPageParams(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	list, total, err := s.svc.CampusCooperation.AdminOrders(ctx.Request().Context(), currentAdminID(ctx), page, size, ctx.QueryParam("status"))
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, map[string]interface{}{"list": list, "total": total, "page": page, "size": size})
}
func (s *AdminServer) SaveCommercialOrder(ctx echo.Context) error {
	id := int64(0)
	if ctx.Param("id") != "" {
		var err error
		id, err = cooperationResourceID(ctx)
		if err != nil {
			return mapServiceError(ctx, err)
		}
	}
	var in service.CommercialOrderInput
	if err := ctx.Bind(&in); err != nil {
		return response.BadRequest(ctx, "商单内容无效")
	}
	id, err := s.svc.CampusCooperation.SaveOrder(ctx.Request().Context(), currentAdminID(ctx), id, in)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, map[string]interface{}{"id": id})
}
func (s *AdminServer) ClaimCommercialOrder(ctx echo.Context) error {
	id, err := cooperationResourceID(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	var in struct {
		SchoolID int `json:"schoolId"`
	}
	if err := ctx.Bind(&in); err != nil || in.SchoolID <= 0 {
		return response.BadRequest(ctx, "请选择接单学校")
	}
	data, err := s.svc.CampusCooperation.Claim(ctx.Request().Context(), currentAdminID(ctx), in.SchoolID, id)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, data)
}
func (s *AdminServer) ListCampusCooperations(ctx echo.Context) error {
	page, size, err := cooperationPageParams(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	list, total, err := s.svc.CampusCooperation.AdminList(ctx.Request().Context(), currentAdminID(ctx), page, size)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, map[string]interface{}{"list": list, "total": total, "page": page, "size": size})
}
func (s *AdminServer) GetCampusCooperation(ctx echo.Context) error {
	id, err := cooperationResourceID(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	data, err := s.svc.CampusCooperation.AdminDetail(ctx.Request().Context(), currentAdminID(ctx), id, false)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, data)
}
func (s *AdminServer) SaveCampusCooperation(ctx echo.Context) error {
	id, err := cooperationResourceID(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	var in service.CooperationContentInput
	if err := ctx.Bind(&in); err != nil {
		return response.BadRequest(ctx, "合作内容无效")
	}
	if err := s.svc.CampusCooperation.SaveContent(ctx.Request().Context(), currentAdminID(ctx), id, in); err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, nil)
}
func (s *AdminServer) ListCampusCooperationApplications(ctx echo.Context) error {
	id, err := cooperationResourceID(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	page, size, err := cooperationPageParams(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	list, total, err := s.svc.CampusCooperation.AdminApplications(ctx.Request().Context(), currentAdminID(ctx), id, page, size)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, map[string]interface{}{"list": list, "total": total, "page": page, "size": size})
}
func (s *AdminServer) ReviewCampusCooperationApplication(ctx echo.Context) error {
	id, err := cooperationResourceID(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	var in struct {
		Status string  `json:"status"`
		Remark *string `json:"remark"`
	}
	if err := ctx.Bind(&in); err != nil {
		return response.BadRequest(ctx, "审核参数无效")
	}
	if err := s.svc.CampusCooperation.Review(ctx.Request().Context(), currentAdminID(ctx), id, in.Status, in.Remark); err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, nil)
}
func (s *AdminServer) UploadCampusCooperationImage(ctx echo.Context) error {
	id, err := cooperationResourceID(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	// Check role/school before parsing the potentially large upload body.
	if _, err := s.svc.CampusCooperation.AdminDetail(ctx.Request().Context(), currentAdminID(ctx), id, true); err != nil {
		return mapServiceError(ctx, err)
	}
	if ctx.Request().ContentLength > 6*1024*1024 {
		return response.BadRequest(ctx, "图片最大5MB")
	}
	ctx.Request().Body = http.MaxBytesReader(ctx.Response().Writer, ctx.Request().Body, 6*1024*1024)
	header, err := ctx.FormFile("file")
	if err != nil {
		return response.BadRequest(ctx, "请选择JPG/PNG图片，最大5MB")
	}
	defer func() {
		if ctx.Request().MultipartForm != nil {
			_ = ctx.Request().MultipartForm.RemoveAll()
		}
	}()
	file, err := header.Open()
	if err != nil {
		return response.BadRequest(ctx, "无法读取图片")
	}
	defer file.Close()
	data, err := s.svc.CampusCooperation.UploadImage(ctx.Request().Context(), currentAdminID(ctx), id, file, header)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, map[string]string{"url": data.URL, "key": data.Key})
}
