package validator

type Validator struct {
	Errors map[string]string
}

func New() *Validator {
	return &Validator{
		Errors: make(map[string]string),
	}
}

func (v *Validator) IsValid() bool {
	return len(v.Errors) == 0
}

func (v *Validator) AddError(key string, errDescription string) {
	if _, ok := v.Errors[key]; ok {
		return
	}
	v.Errors[key] = errDescription
}

func (v *Validator) Check(key string, cond bool, errDescription string) {
	if !cond {
		v.AddError(key, errDescription)
	}
}
