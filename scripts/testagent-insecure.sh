#!/bin/bash
HERE="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT=${HERE}/..

exec ${ROOT}/.build/debug/testagent --connect=tcp://127.0.0.1:5010 --insecure $@