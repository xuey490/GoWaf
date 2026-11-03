package handler

import (
	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
)

// TasksList 任务列表（含上次运行结果与下次运行时间）。
func (s *Server) TasksList(c *gin.Context) {
	if s.Tasks == nil {
		response.Internal(c, "任务运行器未初始化")
		return
	}
	list, err := s.Tasks.List()
	if err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"records": list})
}

// TaskUpdate 修改任务配置：enabled / intervalH / params 均可选。
func (s *Server) TaskUpdate(c *gin.Context) {
	if s.Tasks == nil {
		response.Internal(c, "任务运行器未初始化")
		return
	}
	name := c.Param("name")
	var in struct {
		Enabled   *int           `json:"enabled"`
		IntervalH *int           `json:"intervalH"`
		Params    map[string]any `json:"params"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	if err := s.Tasks.Update(name, in.Enabled, in.IntervalH, in.Params); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "task:"+name)
	response.OK(c, gin.H{"updated": name})
}

// TaskRun 立即执行一次任务。
func (s *Server) TaskRun(c *gin.Context) {
	if s.Tasks == nil {
		response.Internal(c, "任务运行器未初始化")
		return
	}
	name := c.Param("name")
	result, err := s.Tasks.Run(name)
	if err != nil {
		// 任务失败也返回 200，把失败结果带回给面板展示
		response.OK(c, gin.H{"name": name, "result": result, "ok": false})
		return
	}
	c.Set(middleware.CtxAuditObj, "task:"+name)
	response.OK(c, gin.H{"name": name, "result": result, "ok": true})
}
