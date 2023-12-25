package events

type mockEvent struct {
	data      any
	eventType Type
}

func (m *mockEvent) GetType() Type {
	return m.eventType
}

func (m *mockEvent) GetData() any {
	return m.data
}
