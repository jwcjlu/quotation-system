package biz

type DispatchLeasedTask struct {
	TaskID          string
	LeaseID         string
	Attempt         int
	RetryMax        int
	RetryBackoffSec []int
}

type StaleDispatchTask struct {
	DispatchLeasedTask
	FailureReason string
}
