package memory

const (
	BackendKnowledge = "multirag-knowledge"
	BackendNative    = "multirag-memory"
)

type BackendCapabilities struct {
	NativeMaxContextChars            int      `json:"nativeMaxContextChars,omitempty"`
	NativeDefaultMessages            int      `json:"nativeDefaultMessages,omitempty"`
	NativeMaxMessages                int      `json:"nativeMaxMessages,omitempty"`
	Backend                          string   `json:"backend"`
	NativeConfigurationSupported     bool     `json:"nativeConfigurationSupported"`
	NativeStatusSupported            bool     `json:"nativeStatusSupported"`
	NativeForgetSupported            bool     `json:"nativeForgetSupported"`
	NativeDiagnosticsSupported       bool     `json:"nativeDiagnosticsSupported"`
	AutomaticExtractionSupported     bool     `json:"automaticExtractionSupported"`
	ConversationCorrectionsSupported bool     `json:"conversationCorrectionsSupported"`
	MemoryTypes                      []string `json:"memoryTypes"`
	RawOnly                          bool     `json:"rawOnly"`
	CorrectionSupported              bool     `json:"correctionSupported"`
	ForgetSupported                  bool     `json:"forgetSupported"`
	PhysicalDeleteSupported          bool     `json:"physicalDeleteSupported"`
	ConsentRevokeSupported           bool     `json:"consentRevokeSupported"`
	DeactivateSupported              bool     `json:"deactivateSupported"`
}

// Conversation corrections are new dialogue evidence; they do not imply that
// the provider atomically disables all contradictory derived records. The
// separate CorrectionSupported flag governs manual ledger CAS editing.
func (s *Service) Capabilities() BackendCapabilities {
	native := s.cfg.Backend == BackendNative
	auto := native && s.cfg.Native != nil
	types := []string{"raw"}
	if auto {
		types = []string{"raw", "semantic", "episodic", "procedural"}
	}
	caps := BackendCapabilities{Backend: s.cfg.Backend, AutomaticExtractionSupported: auto, ConversationCorrectionsSupported: auto, MemoryTypes: types, RawOnly: native && !auto, CorrectionSupported: !auto, ForgetSupported: !native, PhysicalDeleteSupported: !native, ConsentRevokeSupported: !native, DeactivateSupported: !native}
	_, managementErr := s.nativeManagement()
	caps.NativeConfigurationSupported = managementErr == nil
	caps.NativeStatusSupported = auto && managementErr == nil
	_, forgetAvailable := s.cfg.Native.(NativeForget)
	caps.NativeForgetSupported = auto && forgetAvailable
	_, diagnosticsAvailable := s.cfg.Native.(NativeDiagnostics)
	caps.NativeDiagnosticsSupported = auto && diagnosticsAvailable
	if auto {
		caps.NativeMaxContextChars = 4000
		caps.NativeDefaultMessages = 5
		caps.NativeMaxMessages = 20
	}
	return caps
}
