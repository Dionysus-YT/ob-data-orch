// Package agentwire 定义控制面与 Agent 共享的最小机器协议常量。
// 它不承载业务参数、任务执行或网络实现，避免两端对协议版本产生静默漂移。
package agentwire

// Version 是当前受控 Agent 关联与心跳信封的精确协议版本。
const Version = "agent-v1"
