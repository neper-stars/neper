package race

// Bot race IDs - these correspond to the built-in AI races in Stars!
const (
	BotRaceIDRandom      = "0"
	BotRaceIDRobotoids   = "1"
	BotRaceIDTurindrones = "2"
	BotRaceIDAutomitrons = "3"
	BotRaceIDRototills   = "4"
	BotRaceIDCybertrons  = "5"
	BotRaceIDMacintis    = "6"
)

// ValidBotRaceIDs contains all valid bot race IDs
var ValidBotRaceIDs = map[string]bool{
	BotRaceIDRandom:      true,
	BotRaceIDRobotoids:   true,
	BotRaceIDTurindrones: true,
	BotRaceIDAutomitrons: true,
	BotRaceIDRototills:   true,
	BotRaceIDCybertrons:  true,
	BotRaceIDMacintis:    true,
}

// IsValidBotRaceID returns true if the given race ID is a valid bot race
func IsValidBotRaceID(raceID string) bool {
	return ValidBotRaceIDs[raceID]
}
