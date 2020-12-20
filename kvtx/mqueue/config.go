package kvtx_mqueue

import "time"

// ParsePollDur parses the poll duration and optionally sets a default.
func (c *Config) ParsePollDur(min, def time.Duration) (time.Duration, error) {
	var pollDur time.Duration
	var err error
	if pdStr := c.GetPollDur(); pdStr != "" {
		pollDur, err = time.ParseDuration(pdStr)
		if err != nil {
			pollDur = 0
		}
	}
	if min != 0 {
		if pollDur < min && pollDur != 0 {
			pollDur = min
		}
	}
	if def != 0 && pollDur == 0 {
		pollDur = def
	}
	return pollDur, err
}
