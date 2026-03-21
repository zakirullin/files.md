// Don't want to put this hacky complex code into main bot file.
// The purpose of this file is to collapse a few consecutive incoming messages
// into one file. This is useful when a user forwards a few messages to the bot.
//
// 1) For a single message the flow is the same, no changes are made.
// 2) For any additional messages, we check the time of the last message.
// 3) If it was less than or equal to 1 second ago, we collapse it with the first message in the batch.
// 4) A batch is a sequence of messages with a distance of no more than 1 second between them.
// 5) That’s it.
//
// Suppose we have the following timestamps for incoming messages: 0 0 1 1 1 2 2 2 2 3.
// This is all one batch (distance of no more than 1 second between them).
// So we collapse all messages into the first message of the batch (time=0).
//
// Physically, a user cannot send so many messages manually, so we hypothesize
// that these messages were forwarded. I don’t particularly like this assumption,
// but that's the ugliness of real-world problems reflected into code.
// When a user forwards a few messages from "Saved Messages" dialog
// the messages don't have "is_forwarded" flag, so we can only
// distinguish them by using this time-based heuristic.

package internal

import (
	"sync"
)

var (
	firstMsgIndicies sync.Map
	firstMsgTimes    sync.Map
)

func firstMsgTime(userID int64) (int, bool) {
	time, ok := firstMsgTimes.Load(userID)
	if !ok {
		return 0, false
	}

	return time.(int), true
}

func setFirstMsgTime(userID int64, time int) {
	firstMsgTimes.Store(userID, time)
}

func firstMsgIndex(userID int64) (int, bool) {
	msg, ok := firstMsgIndicies.Load(userID)
	if !ok {
		return 0, false
	}

	return msg.(int), true
}

func setFirstMsgIndex(userID int64, msgIndex int, time int) {
	firstTime, ok := firstMsgTime(userID)
	if ok {
		diff := time - firstTime
		// Sent in exactly same second or second after
		if diff == 0 || diff == 1 {
			return
		}
	}

	firstMsgIndicies.Store(userID, msgIndex)
}

func collapseToMsg(userID int64, time int) (int, bool) {
	firstTime, ok := firstMsgTime(userID)
	if !ok {
		return 0, false
	}

	diff := time - firstTime
	// Sent in exactly same second or second after
	if diff == 0 || diff == 1 {
		return firstMsgIndex(userID)
	}

	return 0, false
}
