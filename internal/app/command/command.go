package command

// Used provide a unique id for every command.
var commandCounter uint64 = 0

type Command struct {
	id   uint64
	name string

	/* APHELION EDIT REMOVAL START - COLLABORATION
	undo, redo func()
	APHELION EDIT REMOVAL END */
	// APHELION EDIT ADDITION START - COLLABORATION
	undo, redo           func()
	undoAsync, redoAsync func(func(error))
	// APHELION EDIT ADDITION END
}

func Make(name string, undo, redo func()) Command {
	commandCounter++
	return Command{
		id:   commandCounter,
		name: name,
		undo: undo,
		redo: redo,
	}
}

// APHELION EDIT ADDITION START - COLLABORATION
func MakeAsync(name string, undo, redo func(func(error))) Command {
	commandCounter++
	return Command{
		id:        commandCounter,
		name:      name,
		undoAsync: undo,
		redoAsync: redo,
	}
}

// APHELION EDIT ADDITION END

func (c Command) ReadableName() string {
	return c.name
}

/* APHELION EDIT REMOVAL START - COLLABORATION
func (c Command) Run() Command {
	c.undo()
	return Command{
		id:   c.id,
		name: c.name,
		undo: c.redo,
		redo: c.undo,
	}
}
APHELION EDIT REMOVAL END */
// APHELION EDIT ADDITION START - COLLABORATION
func (c Command) Run() Command {
	c.undo()
	return c.reversed()
}

func (c Command) RunAsync(complete func(Command, error)) {
	if c.undoAsync == nil {
		c.undo()
		complete(c.reversed(), nil)
		return
	}
	c.undoAsync(func(err error) {
		if err != nil {
			complete(Command{}, err)
			return
		}
		complete(c.reversed(), nil)
	})
}

func (c Command) reversed() Command {
	return Command{
		id:        c.id,
		name:      c.name,
		undo:      c.redo,
		redo:      c.undo,
		undoAsync: c.redoAsync,
		redoAsync: c.undoAsync,
	}
}

// APHELION EDIT ADDITION END
