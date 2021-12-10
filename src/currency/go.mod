module currency

require (
	gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/util/logger v0.0.0
	gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/proto v0.0.0
)

replace (
	gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/util/logger => ../util/logger
	gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/proto => ../proto
)

go 1.17
