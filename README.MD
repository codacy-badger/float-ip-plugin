## Float Ip plugin for rancher

> this plugin is used for small scope but not suit for large scope.

## The Inside Principle

The NIC of the host can hold many IP, add a IP to the NIC and forward all ip package about this ip to container. So the container have the indenpendent and special IP.

The float-ip-plugin can automatically add ip for your container ( by docker lable ), the ip must can be access from other host.

## How to use 

1. Run one instance in each host.
2. add the label "io.rancher.container.floating.ip" to the service container




