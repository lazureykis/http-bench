package main

type Errors struct {
	connect uint32
	read    uint32
	write   uint32
	status  uint32
	timeout uint32
}

type Stats struct {
	count uint64
	limit uint64
	min   uint64
	max   uint64
	data  []uint64
}
