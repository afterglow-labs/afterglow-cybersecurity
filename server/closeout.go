package server

import "strconv"

// closeoutPending applies the opt-in hunt board and the planner's recorded
// campaign branches to task completion. A database error fails closed.
func (e *Engine) closeoutPending(taskID string) (bool, error) {
	id, err := strconv.ParseInt(taskID, 10, 64)
	if err != nil {
		return true, err
	}
	open, err := e.m.pg.CountOpenHuntClasses(id)
	if err != nil {
		return true, err
	}
	untested, err := e.m.pg.CountUntestedBranches(id)
	if err != nil {
		return true, err
	}
	return open > 0 || untested > 0, nil
}
