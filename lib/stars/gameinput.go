package stars

import (
	"bytes"
	_ "embed"
	"text/template"

	"github.com/rs/zerolog"

	"fmt"

	"github.com/neper-stars/neper/models"
)

const windowsSep = "\\"

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func NewGameInput(log *zerolog.Logger, baseDir, sessionID, sessionName string, r models.Ruleset, p []models.SessionPlayerRace) *GameInput {
	g := GameInput{
		log:             log,
		BaseDir:         baseDir,
		SessionID:       sessionID,
		SessionName:     sessionName,
		NumberOfPlayers: len(p),
		RuleSet:         r,
		Players:         p,
	}
	g.computeUniverseFilename()
	g.computeRaces()
	return &g
}

type GameInput struct {
	log              *zerolog.Logger
	BaseDir          string
	SessionID        string
	SessionName      string
	NumberOfPlayers  int
	RuleSet          models.Ruleset
	Players          []models.SessionPlayerRace
	Races            []string
	UniverseFilename string
}

func (g *GameInput) computeUniverseFilename() {
	g.UniverseFilename = g.gameFile("game.xy")
}

func (g *GameInput) computeRaces() {
	for _, p := range g.Players {
		if p.IsBot {
			g.Races = append(g.Races, g.botLine(p))
		} else {
			g.Races = append(g.Races, g.playerLine(p))
		}
	}
}

func (g *GameInput) botLine(pr models.SessionPlayerRace) string {
	return fmt.Sprintf("# %s %d", pr.RaceID, *pr.BotLevel)
}

func (g *GameInput) gameDir() string {
	return g.BaseDir + windowsSep + g.SessionID
}

func (g *GameInput) gameFile(fn string) string {
	return g.gameDir() + windowsSep + fn
}

func (g *GameInput) playerLine(pr models.SessionPlayerRace) string {
	p := g.gameFile(fmt.Sprintf("game.r%d", pr.PlayerOrder+1))
	return p
}

func (g *GameInput) Content() ([]byte, error) {
	tmpl := GameInputTmpl()
	w := new(bytes.Buffer)
	if err := tmpl.Execute(w, g); err != nil {
		g.log.Err(err).Msg("failed to render gameinput template")
		return nil, err
	}
	return w.Bytes(), nil
}

func GameInputTmpl() *template.Template {
	tmpl, err := template.New("gameinput").Funcs(template.FuncMap{
		"starsbool": func(b bool) string {
			if b {
				return "1"
			}
			return "0"
		},
	}).Parse(gameInput)
	must(err)
	return tmpl
}

//go:embed gameinput.tmpl
var gameInput string
