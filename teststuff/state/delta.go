package state

type Delta struct {
	Frame uint64
	// entity -> component -> serialized
	Updated map[Entity]map[string][]byte
	// entity -> component names
	Removed map[Entity][]string
	// entities
	Destroyed []Entity
}

func (w *World) BuildDelta() Delta {
	delta := Delta{
		Frame:     w.frame,
		Updated:   make(map[Entity]map[string][]byte),
		Removed:   make(map[Entity][]string),
		Destroyed: make([]Entity, 0),
	}

	for e, comps := range w.dirtyComponents {
		if _, contains := w.deletedEntities[e]; contains {
			continue
		}

		for name := range comps {
			comp := w.entities[e][name]
			if comp == nil {
				continue
			}

			if comp.isDeleted {
				delta.Removed[e] = append(delta.Removed[e], name)
				delete(w.entities[e], name)
				continue
			}

			data, err := comp.value.Serialize()
			if err != nil {
				continue
			}

			if _, ok := delta.Updated[e]; !ok {
				delta.Updated[e] = make(map[string][]byte)
			}
			delta.Updated[e][name] = data
			comp.seq++
		}
	}

	for eid := range w.deletedEntities {
		delta.Destroyed = append(delta.Destroyed, eid)
		delete(w.entities, eid)
	}

	w.deletedEntities = make(map[Entity]bool)
	w.dirtyComponents = make(map[Entity]map[string]bool)
	w.frame++

	return delta
}
