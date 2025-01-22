#!/bin/bash

exec ./.build/debug/testagent --insecure --connect=tcp://127.0.0.1 $@
