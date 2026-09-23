package orchestrator

// PersonaSessionTerminator exposes core session cleanup to persona features
// such as workspace orchestration, which do not depend on Office.
func (s *Service) PersonaSessionTerminator() *officeSessionTerminator {
	return newOfficeSessionTerminator(s)
}
