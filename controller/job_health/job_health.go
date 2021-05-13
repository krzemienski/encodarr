package job_health

import (
	"context"
	"time"

	"github.com/BrenekH/encodarr/controller"
)

func NewChecker(ds controller.HealthCheckerDataStorer, ss controller.SettingsStorer) Checker {
	return Checker{
		ds: ds,
		ss: ss,

		lastCheckTime: time.Unix(0, 0),
	}
}

type Checker struct {
	ds controller.HealthCheckerDataStorer
	ss controller.SettingsStorer

	lastCheckTime time.Time
}

// Run loops through the provided slice of dispatched jobs and checks if any have
// surpassed the allowed time between updates, if the Health Check timing interval has expired.
func (c *Checker) Run() (uuidsToNull []controller.UUID) {
	if time.Since(c.lastCheckTime) >= time.Duration(c.ss.HealthCheckInterval()) {
		c.lastCheckTime = time.Now()

		djs := c.ds.DispatchedJobs()

		for _, v := range djs {
			if time.Since(v.LastUpdated) >= time.Duration(c.ss.HealthCheckTimeout()) {
				uuidsToNull = append(uuidsToNull, v.UUID)
			}
		}
	}
	return
}

// Start just satisfies the controller.HealthChecker interface.
// There is no implemented functionality.
func (c *Checker) Start(ctx *context.Context) {}
