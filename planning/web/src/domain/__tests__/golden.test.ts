/**
 * golden 向量校验：前端实现必须与 Python 侧逐值一致。
 *
 * 这个文件是整个「允许两份公式实现」策略的保险丝。如果它红了，
 * 说明前后端有一侧算错了 —— 必须停下来查，不能改测试。
 */

import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import {
  classifyO,
  computeLengthChain,
  excel15,
  ladderRound,
  pathUnits,
  toPurchaseTier,
  type LengthRule,
  type OClass,
  type RackPos,
  type Rung,
} from '../cableLength';

interface VectorRack {
  room: string;
  channel: number;
  seq: number;
  side: 'N' | 'S';
  col: number;
  row: number;
}

interface LinkVector {
  name: string;
  a: VectorRack;
  b: VectorRack;
  cableType: string;
  oClass: OClass;
  expect: {
    n: number;
    o: number | null;
    p: number | null;
    q: number | null;
    r: number | null;
    s: number | null;
    t: number | null;
  };
}

interface GoldenFile {
  rule: {
    rackSpacingM: number;
    slackRatio: number;
    nRowCoef: number;
    nRowExtra: number;
    inRackLenByType: Record<string, number>;
    cmaxByRoom: Record<string, number>;
    crossAisleOffset: number;
  };
  rungs: Rung[];
  linkVectors: LinkVector[];
  ladderVectors: { s: number; expect: number | null }[];
  numericVectors: { p: number; raw: number; expect: number }[];
}

const GOLDEN_PATH = resolve(__dirname, '../../../../shared/golden/length_vectors.json');

function loadGolden(): GoldenFile {
  return JSON.parse(readFileSync(GOLDEN_PATH, 'utf-8')) as GoldenFile;
}

function toRackPos(v: VectorRack): RackPos {
  return {
    roomCode: v.room,
    channelNo: v.channel,
    seqNo: v.seq,
    side: v.side,
    col: v.col,
    row: v.row,
  };
}

const golden = loadGolden();

const rule: LengthRule = {
  rackSpacingM: golden.rule.rackSpacingM,
  slackRatio: golden.rule.slackRatio,
  nRowCoef: golden.rule.nRowCoef,
  nRowExtra: golden.rule.nRowExtra,
  inRackLenByType: golden.rule.inRackLenByType,
  cmaxByRoom: golden.rule.cmaxByRoom,
  crossAisleOverrides: {},
  defaultUnknownQ: 5,
};

const rungs: Rung[] = golden.rungs;

describe('golden 契约加载', () => {
  it('向量文件存在且结构完整', () => {
    expect(golden.linkVectors.length).toBeGreaterThan(20);
    expect(golden.ladderVectors.length).toBeGreaterThan(20);
    expect(golden.numericVectors.length).toBeGreaterThan(20);
    expect(rungs.length).toBe(10);
  });

  it('向量覆盖 A/B/C 三类空间关系', () => {
    const classes = new Set(golden.linkVectors.map((v) => v.oClass));
    // 这是防退化断言：如果有人只留 A 类向量，前端 B/C 实现错了也没人发现
    expect([...classes].sort()).toEqual(['A', 'B', 'C']);
  });
});

describe('完整公式链（逐向量比对）', () => {
  for (const v of golden.linkVectors) {
    it(`${v.name}`, () => {
      const a = toRackPos(v.a);
      const b = toRackPos(v.b);

      expect(classifyO(a, b)).toBe(v.oClass);

      const got = computeLengthChain(a, b, v.cableType, rule, rungs);

      expect(got.n).toBe(v.expect.n);
      expect(got.o).toBe(v.expect.o);
      expect(got.p).toBe(v.expect.p);
      expect(got.q).toBe(v.expect.q);
      expect(got.r).toBe(v.expect.r);
      expect(got.s).toBe(v.expect.s);
      expect(got.t).toBe(v.expect.t);
      expect(got.overLimit).toBe(v.expect.t === null && v.expect.s !== null);
    });
  }
});

describe('T 阶梯逐条匹配', () => {
  for (const v of golden.ladderVectors) {
    it(`s=${v.s} → ${v.expect}`, () => {
      expect(ladderRound(v.s, rungs)).toBe(v.expect);
    });
  }

  it('(6,7) 缝隙：6.44 落在 10 而不是 7', () => {
    // 这是源 Excel 的真实行为，49 行受影响。不是 bug，是必须复刻的事实。
    expect(ladderRound(6.44, rungs)).toBe(10);
    expect(ladderRound(7.0, rungs)).toBe(7);
  });
});

describe('Excel 15 位精度', () => {
  for (const v of golden.numericVectors) {
    it(`p=${v.p}`, () => {
      expect(excel15(v.raw)).toBe(v.expect);
    });
  }

  it('P=5 时裸浮点有尾巴，收敛后是 3.6', () => {
    const raw = 5 * 0.6 * 1.2;
    expect(raw).not.toBe(3.6); // 3.5999999999999996
    expect(excel15(raw)).toBe(3.6);
  });
});

describe('pathUnits（经验表键）', () => {
  it('A 类取列差', () => {
    const a: RackPos = { roomCode: '403', channelNo: 1, seqNo: 1, side: 'N', col: 38, row: 3 };
    const b: RackPos = { roomCode: '403', channelNo: 1, seqNo: 3, side: 'N', col: 36, row: 3 };
    expect(pathUnits(a, b, rule)).toBe(2);
  });

  it('B 类取排端折返', () => {
    const a: RackPos = { roomCode: '403', channelNo: 1, seqNo: 1, side: 'N', col: 38, row: 3 };
    const b: RackPos = { roomCode: '403', channelNo: 1, seqNo: 27, side: 'S', col: 38, row: 4 };
    // (38-38) + (38-38) + 1 = 1
    expect(pathUnits(a, b, rule)).toBe(1);
  });
});

describe('采购档位', () => {
  it('35 → 40（35 不单独采购）', () => {
    expect(toPurchaseTier(35)).toBe(40);
  });
  it('10 → 10', () => {
    expect(toPurchaseTier(10)).toBe(10);
  });
});
