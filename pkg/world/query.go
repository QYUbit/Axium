package main

type Query struct {
	include []ComponentID
	exclude []ComponentID
}

func NewQuery() *Query {
	return &Query{
		include: make([]ComponentID, 0),
		exclude: make([]ComponentID, 0),
	}
}

func (q *Query) With(componentType ...ComponentID) *Query {
	q.include = append(q.include, componentType...)
	return q
}

func (q *Query) Without(componentType ...ComponentID) *Query {
	q.exclude = append(q.exclude, componentType...)
	return q
}

func (q *Query) Execute(se *StateEngine) []EntityID {
	resultSlice := se.store.getEntitySlice()
	defer se.store.returnEntitySlice(resultSlice)

	result := *resultSlice

	se.store.archetypes.Range(func(_, value any) bool {
		arch := value.(*Archetype)

		arch.mu.RLock()
		defer arch.mu.RUnlock()

		matching := true
		for _, cid := range q.include {
			if _, ok := arch.offsets[cid]; !ok {
				matching = false
				break
			}
		}

		if !matching {
			return true
		}

		for _, cid := range q.exclude {
			if _, ok := arch.offsets[cid]; ok {
				matching = false
				break
			}
		}

		if matching {
			result = append(result, arch.entities...)
		}

		return true
	})

	// Return a copy since we're returning the slice to the pool
	finalResult := make([]EntityID, len(result))
	copy(finalResult, result)
	return finalResult
}
