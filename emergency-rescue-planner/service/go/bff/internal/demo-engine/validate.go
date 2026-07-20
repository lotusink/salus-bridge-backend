package demo_engine

import "fmt"

func (req *HeartbeatRequest) Validate() error {
	if req.Progress < 0.0 || req.Progress > 1.0 {
		return fmt.Errorf("progress must be between 0.0 and 1.0")
	}
	return nil
}
