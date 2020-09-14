package common

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	// default Readiness values
	defaultReadinessInitialDelaySeconds int32 = 5
	defaultReadinessPeriodSeconds       int32 = 15
	defaultReadinessTimeoutSeconds      int32 = 3
	defaultReadinessFailureThreshold    int32 = 3
	// default Liveness values
	defaultLivenessInitialDelaySeconds int32  = 30
	defaultLivenessPeriodSeconds       int32  = 60
	defaultLivenessTimeoutSeconds      int32  = 3
	defaultLivenessFailureThreshold    int32  = 5
	defaultCommand                     string = "/openstack/healthcheck"
)

const (
	readiness string = "readiness"
	liveness  string = "liveness"
)

type probeType string

// Probe details
type Probe struct {
	// ProbeType, either readiness, or liveness
	ProbeType           probeType
	Command             string
	InitialDelaySeconds int32 // min value 1
	PeriodSeconds       int32 // min value 1
	TimeoutSeconds      int32 // min value 1
	FailureThreshold    int32 // min value 1
}

// GetProbe -
// TODO: move to lib-common
func (p *Probe) GetProbe() *corev1.Probe {

	switch p.ProbeType {
	case "readiness":
		if p.InitialDelaySeconds == 0 {
			p.InitialDelaySeconds = defaultReadinessInitialDelaySeconds
		}
		if p.PeriodSeconds == 0 {
			p.PeriodSeconds = defaultReadinessPeriodSeconds
		}
		if p.TimeoutSeconds == 0 {
			p.TimeoutSeconds = defaultReadinessTimeoutSeconds
		}
		if p.FailureThreshold == 0 {
			p.FailureThreshold = defaultReadinessFailureThreshold
		}
	case "liveness":
		if p.InitialDelaySeconds == 0 {
			p.InitialDelaySeconds = defaultLivenessInitialDelaySeconds
		}
		if p.PeriodSeconds == 0 {
			p.PeriodSeconds = defaultLivenessPeriodSeconds
		}
		if p.TimeoutSeconds == 0 {
			p.TimeoutSeconds = defaultLivenessTimeoutSeconds
		}
		if p.FailureThreshold == 0 {
			p.FailureThreshold = defaultLivenessFailureThreshold
		}
	}

	if p.Command == "" {
		p.Command = defaultCommand
	}

	return &corev1.Probe{
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{
					p.Command,
				},
			},
		},
		InitialDelaySeconds: p.InitialDelaySeconds,
		PeriodSeconds:       p.PeriodSeconds,
		TimeoutSeconds:      p.TimeoutSeconds,
		FailureThreshold:    p.FailureThreshold,
	}
}
