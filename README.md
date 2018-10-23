# testable

`testable` is a code generator for golang that creates a more testable
wrapper around a specified library to make it more testable.

The motivation for this was to find a way to easily test code using
the autogenerated google API clients.

`testable` simply creates a wrapper package for the package it is
pointed at. This wrapper package contains two subpackages, interfaces
and structs implementing the interfaces. The structs' methods just
pass the call through to the wrapped package.

The idea creating the interface library is that you use this for you
method/function parameters. Then, you can manually implement mocks or
use a tool like `gomock` to do it for you.

## Usage

The CLI interface for `testable` is very simple. There are two command
line flags, `-input` and `-output`. `-input` is just the package you
wish to make testable and `-output` is the path of the directory to
put the subpackages in. `-input` is required but `-output` defaults to
the directory `testable` is exectuted in.