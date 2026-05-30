package ioc

// InitTasks 初始化所有后台任务
// NOTE: 新增后台任务（补偿器、消费者等）时在此处注入
func InitTasks() []Task {
	return []Task{}
}
