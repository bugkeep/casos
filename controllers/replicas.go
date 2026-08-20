package controllers

// replicasOrDefault resolves the replica count a request asked for.
//
// The field is a pointer so that "not sent" and "sent as 0" stay distinct. An
// absent field takes Kubernetes' own default of 1; an explicit 0 is honoured,
// because scaling a workload to zero is something an operator does on purpose
// and both the replica control and the create form offer it. Coercing 0 up to 1
// made those controls lie: the UI accepted the change, the API reported success,
// and the workload kept running.
func replicasOrDefault(requested *int32) int32 {
	if requested == nil {
		return 1
	}
	if *requested < 0 {
		return 0
	}
	return *requested
}
