package api

import "fmt"

// 文案与常量的拼接层，便于集中核对 project.md 第 3 节的占位符。

func formatTicket(minutes int, jwt string) string {
	return fmt.Sprintf(textTicketFmt, minutes, jwt)
}

func fmtInstructions(seconds int) string {
	return fmt.Sprintf(textInstructionsFmt, seconds)
}

func fmtNotStarted(seconds int) string {
	return fmt.Sprintf(hintNotStartedFmt, seconds)
}

func fmtActive(count int, order string) string {
	return fmt.Sprintf(hintActiveFmt, count, order)
}

func fmtRejectedHint(kind, order string, seconds int) string {
	return fmt.Sprintf(hintRejectedFmt, kind, order, seconds)
}

func fmtNewRound(seconds, seq, count int) string {
	return fmt.Sprintf(bodyNewRoundFmt, seconds, seq, count)
}

func fmtProgress(seq, count int, order string) string {
	return fmt.Sprintf(bodyProgressFmt, seq, count, order)
}

func fmtDuplicate(seq, count int) string {
	return fmt.Sprintf(bodyDuplicateFmt, seq, count)
}

func fmtRoundAfterTimeout(seconds, seq, count int) string {
	return fmt.Sprintf(bodyRoundAfterTimeoutFmt, seconds, seq, count)
}

func fmtRoundAfterReject(kind string, seconds, seq, count int) string {
	return fmt.Sprintf(bodyRoundAfterRejectFmt, kind, seconds, seq, count)
}

func fmtRejected(kind, order string, seconds int) string {
	return fmt.Sprintf(textRejectedFmt, kind, order, seconds)
}

func formatPass(passToken string, minutes int) string {
	return fmt.Sprintf(textPassFmt, passToken, minutes)
}
