#!/usr/bin/env python3
"""Compatibility entry point; release builds and offline fixtures share one generator."""
import pathlib, subprocess, sys
subprocess.run(['node', str(pathlib.Path(__file__).with_suffix('.cjs')), *sys.argv[1:]], check=True)
