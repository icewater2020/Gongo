package gongo

import (
	"bufio";
	"fmt";
	"io";
	"os";
	"regexp";
	"sort";
	"strconv";
	"strings";
)

// The gongo package handles I/O for Go-playing robots written in Go.

// Go-playing robots are normally implemented as command-line tools that
// take commands from a controller on stdin and write responses to
// stdout. The Go Text Protocol (GTP) [1] defines how this should be done. 
// A robot that implements GTP can be plugged into various useful tools.
// For example, GoGui [2] implements a user interface that will work with
// any robot that implements GTP.
// [1] http://www.lysator.liu.se/~gunnar/gtp/gtp2-spec-draft2/gtp2-spec.html
// [2] http://gogui.sourceforge.net/

// Executes GTP commands until "quit" is received, using the specified robot.
// Returns nil for quit or non nil for I/O error. 
func Run(robot GoRobot, input io.Reader, out io.Writer) os.Error {
	in := bufio.NewReader(input);
	for {
		command, args, err := parseCommand(in);
		if err != nil { return err; }

		next_handler, ok := handlers[command];
		if !ok {
			fmt.Fprint(out, error("unknown command"));
			continue;
		}

		fmt.Fprint(out, next_handler(request{robot, args}));

		if command == "quit" { break; }
	}
	return nil;
}

type GoRobot interface {
	// Attempts to change the board size. If the robot doesn't support the
	// new size, return false. (In any case, board sizes above 25 aren't
	// supported by GTP.)
	// The controller should call ClearBoard next, or the results are undefined. 
	SetBoardSize(size int) (ok bool);

	ClearBoard();
	SetKomi(komi float);

	// Adds a move to the board. Moves can be added in any order, for example
	// to set up a position or replay a game. The robot should automatically
	// handle captures. If a move is illegal, return false.
	Play(move Move) (ok bool);

	// Asks the robot to generate a move at the current position for the given
	// color. The robot may be asked to play either side, regardless of which
	// side it was playing before.
	// The robot should return a vertex (including pass) and handle captures
	// automatically. Or it can resign by returning ok=false.
	GenMove(color Color) (vertex Vertex, ok bool);
}

// Types used by the GoRobot interface

type Color bool;
const (
	Black = false;
	White = true;
)

func ParseColor(input string) (c Color, ok bool) {
	switch strings.ToLower(input) {
	case "w","white": return White, true;
	case "b","black": return Black, true;
	}
	return false, false;
}

func (c Color) String() string {
	switch {
	case c == White: return "White";
	case c == Black: return "Black";
	}
	panic("not reachable");
}

// Identifies a place on the board to play a stone. The zero value is "pass".
// The x index goes from left to right with a range of 1 to the board's size.
// This is printed as letters starting from 'A', skipping 'I'.
// The y index goes from bottom to top, from 1.
type Vertex struct {
	X int; 
	Y int; 
}

const MaxBoardSize = 25;

func ParseVertex(input string) (v Vertex, ok bool) {
	input = strings.ToUpper(input);
	if len(input) < 2 { return Vertex{}, false; }

	if input == "PASS" { return Vertex{}, true; }

	x := 1 + int(input[0]) - int('A');
	if (input[0] > 'I') { x--; }
	if x < 1 || x > MaxBoardSize { return Vertex{}, false; }

	y, err := strconv.Atoi(input[1:len(input)]); 
	if err != nil || y < 1 || y > MaxBoardSize {
		return Vertex{}, false;
	}

	return Vertex{X: x, Y: y}, true;
}

func (v Vertex) IsPass() bool {
	return v.X == 0 && v.Y == 0;
}

func (v Vertex) IsValid(boardSize int) bool {
	return v.IsPass() || (v.X >= 1 && v.X <= boardSize && v.Y >= 1 && v.Y <= boardSize);
}

func (this Vertex) Equals(that Vertex) bool {
	return this.X == that.X && this.Y == that.Y;
}

func (v Vertex) String() string {
	if v.IsPass() {
		return "pass";
	} else if !v.IsValid(MaxBoardSize) {
		return fmt.Sprintf("invalid: (%v,%v)", v.X, v.Y);
	}
	x_letter := byte(v.X) - 1 + 'A';
	if x_letter >= 'I' { x_letter--; }
	return fmt.Sprintf("%c%v", x_letter, v.Y );
}

type Move struct {
	Color Color;
	Vertex Vertex;
}

func ParseMove(input string) (m Move, ok bool) {
	words := strings.Split(input, " ", 0);
	if len(words) != 2 { return Move{}, false; }
	color, ok := ParseColor(words[0]);
	if !ok { return  Move{}, false; }
	vertex, ok := ParseVertex(words[1]);
	if !ok { return Move{}, false; }
	return Move{color, vertex}, true;
}

func (this Move) Equals(that Move) bool {
	return this.Color == that.Color && this.Vertex.Equals(that.Vertex);
}

func (m Move) String() string {
	return fmt.Sprintf("%v %v", m.Color, m.Vertex);
}

// === driver internals ===

var word_regexp = regexp.MustCompile("[^  ]+")

func parseCommand(in *bufio.Reader) (cmd string, args []string, err os.Error) {
	for {
		line, err := in.ReadString('\n');
		if err != nil { return "", nil, err; }
		line = strings.TrimSpace(line);
		if line != "" && line[0] != '#' {
			words := word_regexp.AllMatchesString(line, 0);
			return words[0], words[1:len(words)], nil;
		}
	}
	return "", nil, os.NewError("shouldn't get here");
}

type handler func (request) response;

type request struct {
	robot GoRobot;
	args []string;
}

type response struct {
	message string;
	success bool
}

func success(message string) response {
	return response{message, true}
}

func error(message string) response {
	return response{message, false}
}

func (r response) String() string {
	prefix := "=";
	if !r.success { prefix = "?" }
	return prefix + " " + r.message + "\n\n";
}

var (
	// workaround for issue 292
	_known = func(req request) response { return handle_known_command(req) };
	_list = func(req request) response { return handle_list_commands(req) };

	handlers = map[string] handler {
		"boardsize": handle_boardsize,
		"clear_board": func (req request) response { req.robot.ClearBoard(); return success(""); },
		"genmove": handle_genmove,
		"known_command" : _known,
		"komi": handle_komi,
		"list_commands": _list,
		"name" : func(req request) response { return success("gongo") },
		"play": handle_play,
		"protocol_version" : func(req request) response { return success("2") },
		"quit" : func (req request) response { return success("") },
		"version" : func(req request) response { return success("") },

	};
)

func handle_known_command(req request) response {
	if len(req.args) != 1 { return error("wrong number of arguments"); }

	_, ok := handlers[req.args[0]];
	return success(fmt.Sprint(ok));
}

func handle_list_commands(req request) response {
	if len(req.args) != 0 { return error("wrong number of arguments"); }

	names := make([]string, len(handlers));
	i := 0;
	for name := range handlers {
		names[i] = name;
		i++;
	}

	sort.SortStrings(names);
	return success(strings.Join(names, "\n"));
}

func handle_boardsize(req request) response {
	if len(req.args) != 1 { return error("wrong number of arguments"); }

	size, err := strconv.Atoi(req.args[0]);
	if err != nil { return error("unacceptable size"); }

	if !req.robot.SetBoardSize(size) {
		return error("unacceptable size");
	}

	return success("");
}

func handle_komi(req request) response {
	if len(req.args) != 1 { return error("wrong number of arguments"); }
	
	komi, err := strconv.Atof(req.args[0]);
	if err != nil { return error("syntax error"); }
	
	req.robot.SetKomi(komi);
	return success("");
}

func handle_play(req request) response {
	if len(req.args) != 2 { return error("wrong number of arguments"); }

	move, ok := ParseMove(req.args[0] + " " + req.args[1]);
	if !ok { return error("syntax error"); }

	ok = req.robot.Play(move);
	if !ok { return error("illegal move"); }

	return success("");
}

func handle_genmove(req request) response {
	if len(req.args) != 1 { return error("wrong number of arguments"); }

	color, ok := ParseColor(req.args[0]);
	if !ok { return error("syntax error"); }		

	vertex, ok := req.robot.GenMove(color);
	if !ok { return success("resign"); }

	return success(vertex.String());
}
