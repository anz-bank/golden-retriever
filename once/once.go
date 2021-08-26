package once

import "sync"

type Once struct {
	queue map[string][]chan bool
	mutex *sync.Mutex
}

func NewOnce() Once {
	return Once{make(map[string][]chan bool), &sync.Mutex{}}
}

func (o Once) Register(k string) chan bool {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	_, ok := o.queue[k]

	if ok {
		c := make(chan bool)
		o.queue[k] = append(o.queue[k], c)

		return c
	}

	o.queue[k] = make([]chan bool, 0)

	return nil
}

func (o Once) Unregister(k string) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if _, ok := o.queue[k]; !ok {
		return
	}

	for _, c := range o.queue[k] {
		c <- true
	}
	delete(o.queue, k)
}
