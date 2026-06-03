package membertype

// Resolve determines the MemberType from member flags and space membership.
func Resolve(isCoordinator, isReviewer bool, memberCount int) MemberType {
	if isReviewer {
		return &ReviewerType{}
	}
	if isCoordinator {
		if memberCount <= 1 {
			return &LoneCoordinatorType{}
		}
		return &CoordinatorType{}
	}
	return &WorkerType{}
}
