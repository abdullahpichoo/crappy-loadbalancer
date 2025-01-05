package todosController

type Todo struct {
	UserId    uint32 `json:"userId"`
	ID        uint32 `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

func Index() ([]Todo, error) {
	todos := []Todo{
		{
			UserId:    1,
			ID:        1,
			Title:     "Something",
			Completed: true,
		},
	}
	return todos, nil
}
