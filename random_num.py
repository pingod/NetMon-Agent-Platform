#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import sys
import random

if hasattr(sys.stdout, 'reconfigure'):
    try:
        sys.stdout.reconfigure(encoding='utf-8')
    except Exception:
        pass

def main():
    number = random.randint(1, 39)
    print("=" * 39)
    print(f"随机抽取数字 (范围 1 - 39): 【 {number} 】")
    print("=" * 39)

if __name__ == "__main__":
    main()
