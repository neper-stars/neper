// Package racefiles provides race file processing functionality including
// validation, repair, and password stripping.
package racefiles

import (
	"sync"

	hs "github.com/neper-stars/houston"
	"github.com/rs/zerolog"
)

// ProcessorOptions configures race file processing behavior.
type ProcessorOptions struct {
	StripPassword bool // Remove password from race files
	FixCorrupted  bool // Repair corrupted race files (checksum issues)
}

// Processor handles race file validation and repair.
type Processor struct {
	mu   sync.RWMutex
	opts ProcessorOptions
}

// NewProcessor creates a new race file processor with the given options.
func NewProcessor(opts ProcessorOptions) *Processor {
	return &Processor{opts: opts}
}

// Options returns the current processor options.
func (p *Processor) Options() ProcessorOptions {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.opts
}

// StripPassword returns whether password stripping is enabled.
func (p *Processor) StripPassword() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.opts.StripPassword
}

// FixCorrupted returns whether automatic repair is enabled.
func (p *Processor) FixCorrupted() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.opts.FixCorrupted
}

// Analysis contains the results of analyzing a race file.
type Analysis struct {
	NeedsRepair      bool   // True if the race file has corruption issues
	WasRepaired      bool   // True if the race file was repaired
	RepairError      string // Error message if repair was attempted but failed
	HasPassword      bool   // True if the race file has a password
	PasswordStripped bool   // True if password was removed
}

// Process analyzes and optionally repairs/strips password from race file data.
// It always analyzes the file and logs warnings for corruption.
// Repair and password stripping are performed based on processor options.
// Returns the (possibly modified) data and analysis results.
func (p *Processor) Process(log *zerolog.Logger, data []byte) ([]byte, *Analysis) {
	p.mu.RLock()
	opts := p.opts
	p.mu.RUnlock()

	return ProcessData(log, data, opts)
}

// ProcessData analyzes and optionally repairs/strips password from race file data.
// This is a standalone function that doesn't require a Processor instance.
func ProcessData(log *zerolog.Logger, data []byte, opts ProcessorOptions) ([]byte, *Analysis) {
	analysis := &Analysis{}
	result := data

	// Always analyze race file for corruption
	info, err := hs.AnalyzeRaceFileBytes("", data)
	if err != nil {
		if log != nil {
			log.Warn().Err(err).Msg("failed to analyze race file for corruption")
		}
	} else {
		analysis.NeedsRepair = info.NeedsRepair
		analysis.HasPassword = info.HasPassword

		if info.NeedsRepair {
			if opts.FixCorrupted {
				repaired, repairResult, err := hs.RepairRaceFileBytes(data)
				switch {
				case err != nil:
					analysis.RepairError = err.Error()
				case repairResult.Success:
					analysis.WasRepaired = true
					result = repaired
				default:
					analysis.RepairError = "repair was not successful"
				}
			}
		}
	}

	// Strip password if enabled
	if opts.StripPassword {
		cleaned, stripResult, err := hs.RemoveRacePasswordBytes(result)
		if err != nil {
			if log != nil {
				log.Warn().Err(err).Msg("failed to strip password from race file")
			}
		} else if stripResult.Success && stripResult.PasswordRemoved {
			analysis.PasswordStripped = true
			result = cleaned
		}
	}

	return result, analysis
}

// LogAnalysis logs the analysis results at appropriate levels.
func LogAnalysis(log *zerolog.Logger, analysis *Analysis, context string) {
	if log == nil || analysis == nil {
		return
	}

	logCtx := log.With().Str("context", context).Logger()

	if analysis.NeedsRepair {
		switch {
		case analysis.WasRepaired:
			logCtx.Info().Msg("corrupted race file was automatically repaired")
		case analysis.RepairError != "":
			logCtx.Warn().Str("error", analysis.RepairError).Msg("race file is corrupted and repair failed")
		default:
			logCtx.Warn().Msg("race file is corrupted but automatic repair is disabled")
		}
	}

	if analysis.HasPassword {
		if analysis.PasswordStripped {
			logCtx.Debug().Msg("password was stripped from race file")
		} else {
			logCtx.Debug().Msg("race file has a password")
		}
	}
}
