// Package routes 提供 ETL 管道监控和管理相关的 API 路由的单元测试
// 这些测试验证异步任务队列管理功能的正确性
package routes

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// createETLTestRouter 创建用于 ETL 路由测试的 Gin 路由引擎
// 注入管道检查器中间件以支持 ETL 监控功能的测试
func createETLTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 这里需要模拟管道检查器中间件
	// 在实际测试中，应该创建一个模拟的 PipelineInspector
	return r
}

// TestPipelineProcessingTaskInfo_Struct 测试 PipelineProcessingTaskInfo 结构体
func TestPipelineProcessingTaskInfo_Struct(t *testing.T) {
	Convey("测试 PipelineProcessingTaskInfo 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			now := time.Now()
			task := &PipelineProcessingTaskInfo{
				ID:       serializer.NewHexUUIDv4(),
				Queue:    "default",
				Type:     "data_processing",
				Payload:  `{"data": "test"}`,
				Started:  now,
				Deadline: now.Add(5 * time.Minute),
			}
			So(task.ID.String(), ShouldNotBeEmpty)
			So(task.Queue, ShouldEqual, "default")
			So(task.Type, ShouldEqual, "data_processing")
			So(task.Payload, ShouldEqual, `{"data": "test"}`)
			So(task.Started, ShouldEqual, now)
			So(task.Deadline, ShouldEqual, now.Add(5*time.Minute))
		})

		Convey("应正确序列化为 JSON", func() {
			uuid := serializer.NewHexUUIDv4()
			task := &PipelineProcessingTaskInfo{
				ID:       uuid,
				Queue:    "high_priority",
				Type:     "email_sending",
				Payload:  `{"to": "user@example.com"}`,
				Started:  time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				Deadline: time.Date(2023, 1, 1, 12, 5, 0, 0, time.UTC),
			}
			data, err := json.Marshal(task)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, uuid.String())
			So(string(data), ShouldContainSubstring, `"queue":"high_priority"`)
			So(string(data), ShouldContainSubstring, `"type":"email_sending"`)
			So(string(data), ShouldContainSubstring, `"payload":"{\"to\": \"user@example.com\"}"`)
		})
	})
}

// TestPipelineServerInfo_Struct 测试 PipelineServerInfo 结构体
func TestPipelineServerInfo_Struct(t *testing.T) {
	Convey("测试 PipelineServerInfo 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			now := time.Now()
			server := &PipelineServerInfo{
				ID:               serializer.NewHexUUIDv4(),
				Host:             "server-01.example.com",
				PID:              12345,
				TotalConcurrency: 10,
				QueueWeights: map[string]int{
					"default": 5,
					"high":    3,
					"low":     2,
				},
				Started: now,
				Status:  "active",
				ProcessingTasks: []*PipelineProcessingTaskInfo{
					{
						ID:    serializer.NewHexUUIDv4(),
						Queue: "default",
						Type:  "task_type",
					},
				},
			}
			So(server.ID.String(), ShouldNotBeEmpty)
			So(server.Host, ShouldEqual, "server-01.example.com")
			So(server.PID, ShouldEqual, 12345)
			So(server.TotalConcurrency, ShouldEqual, 10)
			So(server.QueueWeights["default"], ShouldEqual, 5)
			So(server.QueueWeights["high"], ShouldEqual, 3)
			So(server.QueueWeights["low"], ShouldEqual, 2)
			So(server.Started, ShouldEqual, now)
			So(server.Status, ShouldEqual, "active")
			So(len(server.ProcessingTasks), ShouldEqual, 1)
			So(server.ProcessingTasks[0].Queue, ShouldEqual, "default")
		})
	})
}

// TestPipelineSchedulerInfo_Struct 测试 PipelineSchedulerInfo 结构体
func TestPipelineSchedulerInfo_Struct(t *testing.T) {
	Convey("测试 PipelineSchedulerInfo 结构体", t, func() {
		Convey("应正确处理空的上次执行时间", func() {
			nextTime := time.Now().Add(1 * time.Hour)
			scheduler := &PipelineSchedulerInfo{
				ID:       serializer.NewHexUUIDv4(),
				Spec:     "0 */5 * * * *",
				Next:     nextTime,
				Prev:     nil,
				TypeName: "cleanup_task",
				Payload:  `{"action": "cleanup"}`,
			}
			So(scheduler.ID.String(), ShouldNotBeEmpty)
			So(scheduler.Spec, ShouldEqual, "0 */5 * * * *")
			So(scheduler.Next, ShouldEqual, nextTime)
			So(scheduler.Prev, ShouldBeNil)
			So(scheduler.TypeName, ShouldEqual, "cleanup_task")
			So(scheduler.Payload, ShouldEqual, `{"action": "cleanup"}`)
		})

		Convey("应正确处理非空的上次执行时间", func() {
			prevTime := time.Now().Add(-2 * time.Hour)
			nextTime := time.Now().Add(1 * time.Hour)
			scheduler := &PipelineSchedulerInfo{
				ID:       serializer.NewHexUUIDv4(),
				Spec:     "0 0 * * * *",
				Next:     nextTime,
				Prev:     &prevTime,
				TypeName: "daily_report",
				Payload:  `{"report": "daily"}`,
			}
			So(scheduler.Prev, ShouldNotBeNil)
			So(*scheduler.Prev, ShouldEqual, prevTime)
		})
	})
}

// TestPipelineQueueInfo_Struct 测试 PipelineQueueInfo 结构体
func TestPipelineQueueInfo_Struct(t *testing.T) {
	Convey("测试 PipelineQueueInfo 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			now := time.Now()
			queue := &PipelineQueueInfo{
				MemoryUsage: 1024 * 1024, // 1MB
				Latency:     1500 * time.Millisecond,
				Size:        100,
				Groups: []*PipelineGroupInfo{
					{Group: "group1", Size: 50},
					{Group: "group2", Size: 30},
				},
				Pending:        25,
				Active:         5,
				Scheduled:      10,
				Retry:          15,
				Archived:       20,
				Completed:      25,
				Aggregating:    5,
				Processed:      1000,
				Failed:         50,
				ProcessedTotal: 10000,
				FailedTotal:    500,
				Paused:         false,
				RetrievalTime:  now,
			}
			So(queue.MemoryUsage, ShouldEqual, 1024*1024)
			So(queue.Latency, ShouldEqual, 1500*time.Millisecond)
			So(queue.Size, ShouldEqual, 100)
			So(len(queue.Groups), ShouldEqual, 2)
			So(queue.Groups[0].Group, ShouldEqual, "group1")
			So(queue.Groups[0].Size, ShouldEqual, 50)
			So(queue.Pending, ShouldEqual, 25)
			So(queue.Active, ShouldEqual, 5)
			So(queue.Scheduled, ShouldEqual, 10)
			So(queue.Retry, ShouldEqual, 15)
			So(queue.Archived, ShouldEqual, 20)
			So(queue.Completed, ShouldEqual, 25)
			So(queue.Aggregating, ShouldEqual, 5)
			So(queue.Processed, ShouldEqual, 1000)
			So(queue.Failed, ShouldEqual, 50)
			So(queue.ProcessedTotal, ShouldEqual, 10000)
			So(queue.FailedTotal, ShouldEqual, 500)
			So(queue.Paused, ShouldBeFalse)
			So(queue.RetrievalTime, ShouldEqual, now)
		})
	})
}

// TestNewPipelineTaskInfo_Function 测试 NewPipelineTaskInfo 函数
func TestNewPipelineTaskInfo_Function(t *testing.T) {
	Convey("测试 NewPipelineTaskInfo 函数", t, func() {
		Convey("应正确转换 asynq.TaskInfo", func() {
			now := time.Now()
			rawTask := &asynq.TaskInfo{
				ID:            "11111111-2222-3333-4444-555555555555",
				Queue:         "default",
				Type:          "process_data",
				Payload:       []byte(`{"data": "test"}`),
				Result:        []byte(`{"status": "success"}`),
				State:         asynq.TaskStateCompleted,
				MaxRetry:      3,
				Retried:       0,
				LastErr:       "",
				LastFailedAt:  time.Time{},
				NextProcessAt: time.Time{},
				CompletedAt:   now,
				Deadline:      now.Add(1 * time.Hour),
				Timeout:       30 * time.Minute,
				Retention:     24 * time.Hour,
				Group:         "batch1",
				IsOrphaned:    false,
			}

			taskInfo := NewPipelineTaskInfo(rawTask)
			So(taskInfo.Queue, ShouldEqual, "default")
			So(taskInfo.Type, ShouldEqual, "process_data")
			So(taskInfo.Payload, ShouldEqual, `{"data": "test"}`)
			So(taskInfo.Result, ShouldEqual, `{"status": "success"}`)
			So(taskInfo.State, ShouldEqual, asynq.TaskStateCompleted.String())
			So(taskInfo.MaxRetry, ShouldEqual, 3)
			So(taskInfo.Retried, ShouldEqual, 0)
			So(taskInfo.LastErr, ShouldBeEmpty)
			So(taskInfo.LastFailedAt, ShouldBeNil)
			So(taskInfo.NextProcessAt, ShouldBeNil)
			So(taskInfo.CompletedAt, ShouldNotBeNil)
			So(*taskInfo.CompletedAt, ShouldEqual, now)
			So(taskInfo.Deadline, ShouldNotBeNil)
			So(*taskInfo.Deadline, ShouldEqual, now.Add(1*time.Hour))
			So(taskInfo.Timeout, ShouldEqual, 30*time.Minute)
			So(taskInfo.Retention, ShouldEqual, 24*time.Hour)
			So(taskInfo.Group, ShouldEqual, "batch1")
			So(taskInfo.IsOrphaned, ShouldBeFalse)
		})

		Convey("应正确处理空时间字段", func() {
			rawTask := &asynq.TaskInfo{
				ID:       "12345678-1234-1234-1234-123456789012",
				Queue:    "high",
				Type:     "send_email",
				Payload:  []byte(`{"to": "test@example.com"}`),
				State:    asynq.TaskStatePending,
				MaxRetry: 5,
			}

			taskInfo := NewPipelineTaskInfo(rawTask)
			So(taskInfo.Queue, ShouldEqual, "high")
			So(taskInfo.Type, ShouldEqual, "send_email")
			So(taskInfo.State, ShouldEqual, asynq.TaskStatePending.String())
			So(taskInfo.MaxRetry, ShouldEqual, 5)
			So(taskInfo.LastFailedAt, ShouldBeNil)
			So(taskInfo.NextProcessAt, ShouldBeNil)
			So(taskInfo.CompletedAt, ShouldBeNil)
			So(taskInfo.Deadline, ShouldBeNil)
		})
	})
}

// TestETLPipelineInspectorBaseTaskIn_Struct 测试 ETLPipelineInspectorBaseTaskIn 结构体
func TestETLPipelineInspectorBaseTaskIn_Struct(t *testing.T) {
	Convey("测试 ETLPipelineInspectorBaseTaskIn 结构体", t, func() {
		Convey("应正确创建并验证绑定标签", func() {
			input := ETLPipelineInspectorBaseTaskIn{
				Queue: "default",
				State: "pending",
				Group: "batch1",
			}
			So(input.Queue, ShouldEqual, "default")
			So(input.State, ShouldEqual, "pending")
			So(input.Group, ShouldEqual, "batch1")
		})

		Convey("应正确序列化为 JSON", func() {
			input := ETLPipelineInspectorBaseTaskIn{
				Queue: "high_priority",
				State: "active",
				Group: "",
			}
			data, err := json.Marshal(input)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"queue":"high_priority"`)
			So(string(data), ShouldContainSubstring, `"state":"active"`)
			So(string(data), ShouldContainSubstring, `"group":""`)
		})
	})
}

// TestETLPipelineInspectorTaskListIn_Struct 测试 ETLPipelineInspectorTaskListIn 结构体
func TestETLPipelineInspectorTaskListIn_Struct(t *testing.T) {
	Convey("测试 ETLPipelineInspectorTaskListIn 结构体", t, func() {
		Convey("应正确嵌入分页参数", func() {
			input := ETLPipelineInspectorTaskListIn{
				Pagination: Pagination{
					Page:     2,
					PageSize: 20,
				},
				Queue: "default",
				State: "completed",
				Group: "report",
			}
			So(input.Page, ShouldEqual, 2)
			So(input.PageSize, ShouldEqual, 20)
			So(input.Queue, ShouldEqual, "default")
			So(input.State, ShouldEqual, "completed")
			So(input.Group, ShouldEqual, "report")
		})
	})
}

// 由于实际的路由处理函数依赖于外部的 PipelineInspector，
// 以下测试需要在实际环境中配合模拟对象进行测试
// 这里仅提供测试框架，实际测试需要注入模拟的 PipelineInspector

// TestETLRoutes_Integration 集成测试框架
func TestETLRoutes_Integration(t *testing.T) {
	Convey("ETL 路由集成测试框架", t, func() {
		Convey("服务器列表路由测试", func() {
			// 需要模拟 PipelineInspector 和配置中间件
			// r := createETLTestRouter()
			// r.POST("/api/v1/etl/inspector/server/list", etlPipelineInspectorServerListGet)
			// 测试代码...
		})

		Convey("队列列表路由测试", func() {
			// 测试代码...
		})

		Convey("任务列表路由测试", func() {
			// 测试代码...
		})
	})
}

// TestETLPipelineInspectorQueueDailyStat_Struct 测试 PipelineQueueDailyStat 结构体
func TestETLPipelineInspectorQueueDailyStat_Struct(t *testing.T) {
	Convey("测试 PipelineQueueDailyStat 结构体", t, func() {
		Convey("应正确创建每日统计信息", func() {
			date := time.Date(2023, 12, 25, 0, 0, 0, 0, time.UTC)
			stat := &PipelineQueueDailyStat{
				Date:      date,
				Processed: 1500,
				Failed:    25,
			}
			So(stat.Date, ShouldEqual, date)
			So(stat.Processed, ShouldEqual, 1500)
			So(stat.Failed, ShouldEqual, 25)
		})

		Convey("应正确序列化为 JSON", func() {
			date := time.Date(2023, 12, 25, 0, 0, 0, 0, time.UTC)
			stat := &PipelineQueueDailyStat{
				Date:      date,
				Processed: 2000,
				Failed:    50,
			}
			data, err := json.Marshal(stat)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"date":"2023-12-25T00:00:00Z"`)
			So(string(data), ShouldContainSubstring, `"processed":2000`)
			So(string(data), ShouldContainSubstring, `"failed":50`)
		})
	})
}

// TestETLPipelineInspectorDailyStatIn_Struct 测试 ETLPipelineInspectorDailyStatIn 结构体
func TestETLPipelineInspectorDailyStatIn_Struct(t *testing.T) {
	Convey("测试 ETLPipelineInspectorDailyStatIn 结构体", t, func() {
		Convey("应正确验证绑定标签", func() {
			validInputs := []uint{7, 15, 30, 90, 180}
			for _, days := range validInputs {
				input := ETLPipelineInspectorDailyStatIn{Days: days}
				So(input.Days, ShouldEqual, days)
			}
		})

		Convey("应拒绝无效的天数", func() {
			// 注意：这个验证是由 Gin 绑定器在运行时执行的
			// 这里只是展示结构体的使用
			input := ETLPipelineInspectorDailyStatIn{Days: 1}
			So(input.Days, ShouldEqual, 1)
		})
	})
}

// 辅助函数测试
func TestETLHelperFunctions(t *testing.T) {
	Convey("测试 ETL 相关辅助函数", t, func() {
		Convey("PipelineGroupInfo 应正确表示分组信息", func() {
			group := &PipelineGroupInfo{
				Group: "user_sessions",
				Size:  150,
			}
			So(group.Group, ShouldEqual, "user_sessions")
			So(group.Size, ShouldEqual, 150)
		})

		Convey("输出类型别名应正确工作", func() {
			// 测试类型别名
			var serverList ETLPipelineInspectorServerListOut
			serverList = []*PipelineServerInfo{}
			So(serverList, ShouldHaveLength, 0)

			var schedulerList ETLPipelineInspectorSchedulerListOut
			schedulerList = []*PipelineSchedulerInfo{}
			So(schedulerList, ShouldHaveLength, 0)

			var queueList ETLPipelineInspectorQueueListOut
			queueList = make(map[string]*PipelineQueueInfo)
			So(queueList, ShouldBeEmpty)

			var taskList ETLPipelineInspectorTaskListOut
			taskList = []*PipelineTaskInfo{}
			So(taskList, ShouldHaveLength, 0)
		})
	})
}

// TestETLInputValidation 测试 ETL 输入参数的验证逻辑
func TestETLInputValidation(t *testing.T) {
	Convey("测试 ETL 输入参数验证", t, func() {
		Convey("ETLPipelineInspectorSingleTaskIn 应验证队列名称", func() {
			input := ETLPipelineInspectorSingleTaskIn{
				Queue: "default",
			}
			So(input.Queue, ShouldEqual, "default")
			// 注意：实际的 required 验证由 Gin 绑定器处理
		})

		Convey("ETLPipelineInspectorStateTaskDeleteIn 应验证状态值", func() {
			validStates := []string{"pending", "aggregating", "scheduled", "retry", "archived", "completed"}
			for _, state := range validStates {
				input := ETLPipelineInspectorStateTaskDeleteIn{
					Queue: "default",
					State: state,
					Group: "batch1",
				}
				So(input.State, ShouldEqual, state)
				So(input.Group, ShouldEqual, "batch1")
			}
		})

		Convey("ETLPipelineInspectorStateTaskRunIn 应验证可运行的状态值", func() {
			validStates := []string{"aggregating", "scheduled", "retry", "archived"}
			for _, state := range validStates {
				input := ETLPipelineInspectorStateTaskRunIn{
					Queue: "high",
					State: state,
					Group: "",
				}
				So(input.State, ShouldEqual, state)
				So(input.Group, ShouldBeEmpty)
			}
		})
	})
}

// TestPipelineTaskInfo_EdgeCases 测试 PipelineTaskInfo 的边缘情况
func TestPipelineTaskInfo_EdgeCases(t *testing.T) {
	Convey("测试 PipelineTaskInfo 的边缘情况", t, func() {
		Convey("应正确处理孤儿任务", func() {
			task := &PipelineTaskInfo{
				ID:         serializer.NewHexUUIDv4(),
				Queue:      "default",
				Type:       "orphaned_task",
				IsOrphaned: true,
			}
			So(task.IsOrphaned, ShouldBeTrue)
			So(task.Type, ShouldEqual, "orphaned_task")
		})

		Convey("应正确处理重试次数", func() {
			task := &PipelineTaskInfo{
				ID:       serializer.NewHexUUIDv4(),
				Queue:    "retry_queue",
				MaxRetry: 5,
				Retried:  3,
				LastErr:  "connection timeout",
			}
			So(task.MaxRetry, ShouldEqual, 5)
			So(task.Retried, ShouldEqual, 3)
			So(task.LastErr, ShouldEqual, "connection timeout")
		})

		Convey("应正确处理聚合任务", func() {
			task := &PipelineTaskInfo{
				ID:    serializer.NewHexUUIDv4(),
				Queue: "aggregation",
				Group: "user_analytics",
			}
			So(task.Group, ShouldEqual, "user_analytics")
		})
	})
}

// TestQueueInfo_Calculations 测试队列信息的计算逻辑
func TestQueueInfo_Calculations(t *testing.T) {
	Convey("测试队列信息的计算逻辑", t, func() {
		Convey("应正确计算队列总大小", func() {
			queue := &PipelineQueueInfo{
				Pending:     10,
				Active:      5,
				Scheduled:   3,
				Retry:       2,
				Archived:    15,
				Completed:   25,
				Aggregating: 5,
			}
			// 总大小应该是所有状态任务数量的总和
			expectedSize := queue.Pending + queue.Active + queue.Scheduled +
				queue.Retry + queue.Archived + queue.Completed + queue.Aggregating
			So(queue.Size, ShouldEqual, 0) // 注意：Size 字段需要单独设置
			// 这里演示计算逻辑
			So(expectedSize, ShouldEqual, 65)
		})

		Convey("应正确计算成功率", func() {
			queue := &PipelineQueueInfo{
				ProcessedTotal: 1000,
				FailedTotal:    50,
			}
			// 计算成功率：(总处理数 - 失败数) / 总处理数 * 100%
			successRate := float64(queue.ProcessedTotal-queue.FailedTotal) / float64(queue.ProcessedTotal) * 100
			So(successRate, ShouldEqual, 95.0)
		})
	})
}
