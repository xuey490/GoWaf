package handler

import (
	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
)

// TasksList 返回计划任务列表，含上次运行结果与下次运行时间。
// 路由：GET /api/tasks（需登录）。
// 参数 c：Gin 上下文（无查询参数）。
// 返回：无（200 返回 {"records":[...]}；任务运行器未初始化或数据库失败返回 500）。
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

// TaskUpdate 修改任务配置：enabled / intervalH / params 均可选，只更新传入的字段。
// 路由：PUT /api/tasks/:name（写操作组：CSRF 校验、演示只读拦截、审计、settings:write 权限）。
// 参数 c：Gin 上下文，路径参数 name 为任务名，JSON 请求体携带要更新的字段。
// 返回：无（200 返回 {"updated":name}；参数格式错误或任务不存在/参数非法返回 400，运行器未初始化返回 500）。
func (s *Server) TaskUpdate(c *gin.Context) {
	if s.Tasks == nil {
		response.Internal(c, "任务运行器未初始化")
		return
	}
	name := c.Param("name")
	var in struct {
		Enabled   *int           `json:"enabled"`   // 是否启用：指针类型，不传保持原值
		IntervalH *int           `json:"intervalH"` // 运行间隔（小时）：指针类型，不传保持原值
		Params    map[string]any `json:"params"`    // 任务参数：不传保持原值
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	if err := s.Tasks.Update(name, in.Enabled, in.IntervalH, in.Params); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	// 把操作对象写入请求上下文，供审计中间件落库
	c.Set(middleware.CtxAuditObj, "task:"+name)
	response.OK(c, gin.H{"updated": name})
}

// TaskRun 立即执行一次指定任务，并把执行结果返回给面板。
// 路由：POST /api/tasks/:name/run（写操作组：CSRF 校验、演示只读拦截、审计、settings:write 权限）。
// 参数 c：Gin 上下文，路径参数 name 为任务名。
// 返回：无（200 返回 name/result/ok；任务执行失败时 ok=false 仍返回 200，运行器未初始化返回 500）。
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
