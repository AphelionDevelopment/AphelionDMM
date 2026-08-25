package command

// Used provide a unique id for every command.
var commandCounter uint64 = 0

type Command struct {
	id   uint64
	name string

	undo, redo           func()
	undoAsync, redoAsync func(func(error))
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

func MakeAsync(name string, undo, redo func(func(error))) Command {
	commandCounter++
	return Command{
		id:        commandCounter,
		name:      name,
		undoAsync: undo,
		redoAsync: redo,
	}
}

func (c Command) ReadableName() string {
	return c.name
}

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
