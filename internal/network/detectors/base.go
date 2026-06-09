package detectors

import (
	n "zhongkui.com/zhongkui/internal/models/network"
)

// sortedFrameStack 返回按时间戳排序的帧栈，合并客户端和服务端的帧信息。
func sortedFrameStack(reqFrameStack, rspFrameStack n.FrameStack) n.FrameStack {
	fs := make(n.FrameStack, 0, len(reqFrameStack)+len(rspFrameStack))
	clientIndex, serverIndex := 0, 0
	for {
		if clientIndex >= len(reqFrameStack) {
			fs = append(fs, rspFrameStack[serverIndex:]...)
			break
		}
		if serverIndex >= len(rspFrameStack) {
			fs = append(fs, reqFrameStack[clientIndex:]...)
			break
		}
		clientFrameInfo := reqFrameStack[clientIndex]
		serverFrameInfo := rspFrameStack[serverIndex]
		if clientFrameInfo.Timestamp.Before(serverFrameInfo.Timestamp) {
			fs = append(fs, clientFrameInfo)
			clientIndex++
		} else {
			fs = append(fs, serverFrameInfo)
			serverIndex++
		}
	}
	return fs
}
