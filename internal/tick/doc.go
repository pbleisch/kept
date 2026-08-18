// Package tick is the loop: gather what is due, evaluate, record, then decide
// what to do about anything with standing and without truth.
//
// No model runs here. Gathering, evaluating, ranking and the
// act/wait/suppress/escalate decision are deterministic, because a queue that
// cannot be explained is worse than a queue that is occasionally wrong.
package tick
