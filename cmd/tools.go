package main

import (
	"gogogot/infra/scheduler"
	"gogogot/infra/tools"
	"gogogot/infra/tools/system"
	"gogogot/infra/tools/web"
)

func coreTools(braveAPIKey string, sched *scheduler.Scheduler) []tools.Tool {
	var all []tools.Tool
	all = append(all, system.BashTool())
	all = append(all, system.FileTools()...)
	all = append(all, system.EditFileTool())
	all = append(all, web.WebSearchTool(braveAPIKey))
	all = append(all, web.WebFetchTool())
	all = append(all, web.WebRequestTool())
	all = append(all, web.WebDownloadTool())
	all = append(all, system.IdentityTools()...)
	all = append(all, system.MemoryTools()...)
	all = append(all, system.SkillTools()...)
	all = append(all, system.SystemInfoTool())
	all = append(all, system.ScheduleTools(sched)...)
	return all
}
