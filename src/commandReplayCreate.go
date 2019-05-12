/*
	Sent when the user clicks on the "Watch Replay", "Share Replay",
	or "Watch Specific Replay" button
	(the client will send a "hello" message after getting "gameStart")

	"data" example:
	{
		source: 'id',
		gameID: 15103, // Only if source is "id"
		json: '{"actions"=[],"deck"=[]}', // Only if source is "json"
		visibility: 'solo',
	}
*/

package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Zamiell/hanabi-live/src/models"
	melody "gopkg.in/olahol/melody.v1"
)

func commandReplayCreate(s *Session, d *CommandData) {
	// Validate the visibility
	if d.Visibility != "solo" && d.Visibility != "shared" {
		s.Warning("That is not a valid replay visibility.")
		return
	}

	// Validate the source
	if d.Source == "id" {
		replayID(s, d)
	} else if d.Source == "json" {
		replayJSON(s, d)
	} else {
		s.Warning("That is not a valid replay source.")
	}
}

func replayID(s *Session, d *CommandData) {
        gameID := d.GameID
	// Check to see if the game exists in the database
	if exists, err := db.Games.Exists(gameID); err != nil {
		log.Error("Failed to check to see if game "+strconv.Itoa(gameID)+" exists:", err)
		s.Error("Failed to initialize the game. Please contact an administrator.")
		return
	} else if !exists {
		s.Warning("Game #" + strconv.Itoa(gameID) + " does not exist in the database.")
		return
	}

	// Convert the game data from the database into a normal table object
	t, success := convertDatabaseGametoTable(s, d)
	if !success {
		return
	}
	tables[t.ID] = t
	log.Info("User \"" + s.Username() + "\" created a new " + d.Visibility + " replay: #" + strconv.Itoa(gameID))
	// (a "table" message will be sent when the user joins)

	// Join the user to the new replay
	d.TableID = t.ID
	commandTableSpectate(s, d)

	// Start the idle timeout
	go t.CheckIdle()
}

func convertDatabaseGametoTable(s *Session, d *CommandData) (*Table, bool) {
	// Get a new table ID
	tableID := newTableID
	newTableID++
        gameID := d.GameID

	// Define a standard naming scheme for shared replays
	name := strings.Title(d.Visibility) + " replay for game #" + strconv.Itoa(gameID)

	// Figure out whether this table should be invisible
	visible := false
	if d.Visibility == "shared" {
		visible = true
	}

	// Get the options from the database
	var options models.Options
	if v, err := db.Games.GetOptions(gameID); err != nil {
		log.Error("Failed to get the options from the database for game "+strconv.Itoa(gameID)+":", err)
		s.Error("Failed to initialize the game. Please contact an administrator.")
		return nil, false
	} else {
		options = v
	}

	// Get the players from the database
	var dbPlayers []*models.Player
	if v, err := db.Games.GetPlayers(d.GameID); err != nil {
		log.Error("Failed to get the players from the database for game "+strconv.Itoa(gameID)+":", err)
		s.Error("Failed to initialize the game. Please contact an administrator.")
		return nil, false
	} else {
		dbPlayers = v
	}

	// Get the notes from the database
	var notes []models.NoteList
	if v, err := db.Games.GetNotes(gameID); err != nil {
		log.Error("Failed to get the notes from the database "+
			"for game "+strconv.Itoa(gameID)+":", err)
		s.Error("Failed to initialize the game. Please contact an administrator.")
		return nil, false
	} else {
		notes = v
	}

	// Convert the database player objects to normal player objects
	players := make([]*Player, 0)
	for i, p := range dbPlayers {
		player := &Player{
			ID:                p.ID,
			Name:              p.Name,
			Notes:             notes[i].Notes,
			Character:         charactersID[p.CharacterAssignment],
			CharacterMetadata: p.CharacterMetadata,
		}
		players = append(players, player)
	}

	// Get the actions from the database
	var actionStrings []string
	if v, err := db.GameActions.GetAll(gameID); err != nil {
		log.Error("Failed to get the actions from the database "+
			"for game "+strconv.Itoa(gameID)+":", err)
		s.Error("Failed to initialize the game. Please contact an administrator.")
		return nil, false
	} else {
		actionStrings = v
	}

	// Convert the database action objects to normal action objects
	actions := make([]interface{}, 0)
	for _, actionString := range actionStrings {
		// Convert it from JSON
		var action interface{}
		if err := json.Unmarshal([]byte(actionString), &action); err != nil {
			log.Error("Failed to unmarshal an action:", err)
			s.Error("Failed to initialize the game. Please contact an administrator.")
			return nil, false
		}
		actions = append(actions, action)
	}

	// Get the number of turns from the database
	var numTurns int
	if v, err := db.Games.GetNumTurns(gameID); err != nil {
		log.Error("Failed to get the number of turns from the database for game "+strconv.Itoa(gameID)+":", err)
		s.Error("Failed to initialize the game. Please contact an administrator.")
		return nil, false
	} else {
		numTurns = v
	}

	// Create the table object
	t := &Table{
		ID:         tableID,
		Name:       name,
		Owner:      s.UserID(),
		Visible:    visible,

                Game: &Game{
                        ID: gameID,
                        Running:            true,
                        Replay:             true,
                        DatetimeStarted:    time.Now(),
                        Actions:            actions,
                        EndTurn:            numTurns,
                        HypoActions:        make([]string, 0),
                },
                GameSpec:   &GameSpec{
                        Options: &Options{
                                Variant:              variantsID[options.Variant],
                                Timed:                options.Timed,
                                BaseTime:             options.BaseTime,
                                TimePerTurn:          options.TimePerTurn,
                                Speedrun:             options.Speedrun,
                                DeckPlays:            options.DeckPlays,
                                EmptyClues:           options.EmptyClues,
                                CharacterAssignments: options.CharacterAssignments,
                        },
                        Players:            players,
                },

		Spectators:         make([]*Spectator, 0),
		DisconSpectators:   make(map[int]bool),
		DatetimeCreated:    time.Now(),
		DatetimeLastAction: time.Now(),
		Chat:               make([]*TableChatMessage, 0),
		ChatRead:           make(map[int]int),
	}

	return t, true
}

type GameJSON struct {
	Actions     []ActionJSON `json:"actions"`
	Deck        []CardSimple `json:"deck"`
	FirstPlayer int          `json:"firstPlayer"`
	Notes       [][]string   `json:"notes"`
	Players     []string     `json:"players"`
	Variant     string       `json:"variant"`
}
type ActionJSON struct {
	Clue   Clue `json:"clue"`
	Target int  `json:"target"`
	Type   int  `json:"type"`
}

func replayJSON(s *Session, d *CommandData) {
	// Validate that there is at least one action
	if len(d.GameJSON.Actions) < 1 {
		s.Warning("There must be at least one table action in the JSON array.")
		return
	}

	// Validate actions
	for i, action := range d.GameJSON.Actions {
		if action.Type == actionTypeClue {
			if action.Target < 0 || action.Target > len(d.GameJSON.Players)-1 {
				s.Warning("Action " + strconv.Itoa(i) + " is a clue with an invalid target: " + strconv.Itoa(action.Target))
				return
			}
			if action.Clue.Type < 0 || action.Clue.Type > 1 {
				s.Warning("Action " + strconv.Itoa(i) + " is a clue with an clue type: " + strconv.Itoa(action.Clue.Type))
				return
			}
		} else if action.Type == actionTypePlay || action.Type == actionTypeDiscard {
			if action.Target < 0 || action.Target > len(d.GameJSON.Deck)-1 {
				s.Warning("Action " + strconv.Itoa(i) + " is a play or discard with an invalid target.")
				return
			}
		} else {
			s.Warning("Action " + strconv.Itoa(i) + " has an invalid type: " + strconv.Itoa(action.Type))
			return
		}
	}

	// Validate the variant
	var variant *Variant
	if v, ok := variants[d.GameJSON.Variant]; !ok {
		s.Warning("That is not a valid variant.")
		return
	} else {
		variant = v
	}

	// Validate that the deck contains the right amount of cards
	totalCards := 0
	for _, suit := range variant.Suits {
		if suit.OneOfEach {
			totalCards += 5
		} else {
			totalCards += 10
		}
	}
	if strings.HasPrefix(variant.Name, "Up or Down") {
		totalCards -= len(variant.Suits)
	}
	if len(d.GameJSON.Deck) != totalCards {
		s.Warning("The deck must have " + strconv.Itoa(totalCards) + " cards in it.")
		return
	}

	// Validate the amount of players
	if len(d.GameJSON.Players) < 2 || len(d.GameJSON.Players) > 6 {
		s.Warning("The number of players must be between 2 and 6.")
		return
	}

	// Validate the notes
	if len(d.GameJSON.Notes) == 0 {
		// They did not provide any notes, so create a blank note array
		d.GameJSON.Notes = make([][]string, len(d.GameJSON.Players))
		for i := 0; i < len(d.GameJSON.Players); i++ {
			d.GameJSON.Notes[i] = make([]string, totalCards)
		}
	} else if len(d.GameJSON.Notes) != len(d.GameJSON.Players) {
		s.Warning("The number of note arrays must match the number of players.")
		return
	}

	// Convert the JSON table to a normal table object
	t := convertJSONGametoTable(s, d)
	tables[t.ID] = t
	log.Info("User \"" + s.Username() + "\" created a new " + d.Visibility + " JSON replay: #" + strconv.Itoa(t.ID))
	// (a "table" message will be sent when the user joins)

	// Send messages from fake players to emulate the tableplay that occurred in the JSON actions
	emulateJSONActions(t, d)

	// Do a mini-version of the steps in the "t.End()" function
	t.Game.Replay = true
	t.Game.Turn = 0 // We want to start viewing the replay at the beginning, not the end
	t.Game.EndTurn = t.Game.Turn
	t.Game.Progress = 100

	// Join the user to the new shared replay
	d.TableID = t.ID
	commandTableSpectate(s, d)

	// Start the idle timeout
	go t.CheckIdle()
}

func convertJSONGametoTable(s *Session, d *CommandData) *Table {
	// Get a new table ID
	tableID := newTableID
	newTableID++

	// Define a standard naming scheme for shared replays
	name := strings.Title(d.Visibility) + " replay for JSON table #" + strconv.Itoa(tableID)

	// Figure out whether this table should be invisible
	visible := false
	if d.Visibility == "shared" {
		visible = true
	}

	// Convert the JSON player objects to a normal player objects
	players := make([]*Player, 0)
	for i, name := range d.GameJSON.Players {
		// Prepare the player session that will be used for emulation
		keys := make(map[string]interface{})
		keys["ID"] = -1 // We set it to -1 since it does not matter
		// This can be any arbitrary unique number,
		// but we will set it to the same value as the player index for simplicity
		keys["userID"] = i
		keys["username"] = name
		keys["admin"] = false
		keys["firstTimeUser"] = false
		keys["currentTable"] = tableID
		keys["status"] = statusPlaying

		player := &Player{
			ID:    i,
			Name:  name,
			Index: i,
			Session: &Session{
				&melody.Session{
					Keys: keys,
				},
			},
			Present: true,
			Notes:   d.GameJSON.Notes[i],
		}
		players = append(players, player)
	}

	// Create the table object
	t := &Table{
		ID:      tableID,
		Name:    name,
		Owner:   s.UserID(),
		Visible: visible,
                GameSpec: &GameSpec{
                        Options: &Options{
                                Variant:    d.GameJSON.Variant,
                                EmptyClues: true, // Always enable empty clues in case it is used in the JSON
                        },
                        Players:            players,
                },
		Spectators:         make([]*Spectator, 0),
		DisconSpectators:   make(map[int]bool),
		DatetimeCreated:    time.Now(),
		DatetimeLastAction: time.Now(),
                NoDatabase:         true,
                Game: &Game{
                        Running:            true,
                        DatetimeStarted:    time.Now(),
                        Stacks:             make([]int, len(variants[d.GameJSON.Variant].Suits)),
                        StackDirections:    make([]int, len(variants[d.GameJSON.Variant].Suits)),
                        ActivePlayer:       d.GameJSON.FirstPlayer,
                        Clues:              maxClues,
                        MaxScore:           len(variants[d.GameJSON.Variant].Suits) * 5,
                        Actions:            make([]interface{}, 0),
                        HypoActions:        make([]string, 0),
                        EndTurn:            -1,
                },
		Chat:               make([]*TableChatMessage, 0),
		ChatRead:           make(map[int]int),
	}

	// Convert the JSON deck to a normal deck
	t.Game.Deck = make([]*Card, 0)
	for i, c := range d.GameJSON.Deck {
		t.Game.Deck = append(t.Game.Deck, &Card{
			Suit:  c.Suit,
			Rank:  c.Rank,
			Order: i,
		})

	}

	/*
		The following code is mostly copied from the "commandGameStart()" function
	*/

	// Deal the cards
	handSize := t.GameSpec.GetHandSize()
	for _, p := range t.GameSpec.Players {
		for i := 0; i < handSize; i++ {
			p.DrawCard(t)
		}
	}

	// Record the initial status of the table
	t.NotifyStatus(false) // The argument is "doubleDiscard"

	// Show who goes first
	// (this must be sent before the "turn" message
	// so that the text appears on the first turn of the replay)
	text := t.GameSpec.Players[t.Game.ActivePlayer].Name + " goes first"
	t.Game.Actions = append(t.Game.Actions, ActionText{
		Type: "text",
		Text: text,
	})
	log.Info(t.GetName() + text)

	// Record a message about the first turn
	t.NotifyTurn()

	return t
}

func emulateJSONActions(t *Table, d *CommandData) {
	// Emulate the JSON actions to normal action objects
	for _, action := range d.GameJSON.Actions {
		p := t.GameSpec.Players[t.Game.ActivePlayer]
		if action.Type == actionTypeClue {
			commandAction(p.Session, &CommandData{
				Type:   actionTypeClue,
				Target: action.Target,
				Clue:   action.Clue,
			})
		} else if action.Type == actionTypePlay {
			commandAction(p.Session, &CommandData{
				Type:   actionTypePlay,
				Target: action.Target,
			})
		} else if action.Type == actionTypeDiscard {
			commandAction(p.Session, &CommandData{
				Type:   actionTypeDiscard,
				Target: action.Target,
			})
		}
	}
}
