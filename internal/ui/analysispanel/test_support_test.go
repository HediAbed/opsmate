package analysispanel

type errStub string

func (err errStub) Error() string {
	return string(err)
}
