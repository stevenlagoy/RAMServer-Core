package testutil

type MockSender struct {
	Messages [][]byte
}

func (m *MockSender) Send(msg []byte) {
	m.Messages = append(m.Messages, msg)
}
