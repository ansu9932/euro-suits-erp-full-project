import {test} from 'node:test';import assert from 'node:assert/strict';
import {convertMeasurement} from '../src/lib/materialMeasurements.ts';
test('GSM and inch width convert to grams per linear metre',()=>{const m=convertMeasurement({weight:200,width:60,weight_unit:'GSM',width_unit:'inch'});assert.equal(m.widthCm,152.4);assert.ok(Math.abs(m.gramsPerMetre-304.8)<1e-8)});
test('linear-metre weight needs a positive width',()=>{assert.equal(convertMeasurement({weight:300,width:0,weight_unit:'g/linear metre',width_unit:'cm'}).gsm,null);assert.equal(convertMeasurement({weight:300,width:150,weight_unit:'g/linear metre',width_unit:'cm'}).gsm,200)});
test('ounce per square yard conversion',()=>{assert.ok(Math.abs(convertMeasurement({weight:1,width:1,weight_unit:'oz/yd²',width_unit:'metre'}).gsm-33.905747)<1e-7)});
test('zero values remain finite',()=>{const m=convertMeasurement({});assert.equal(m.gsm,0);assert.equal(m.widthCm,0)});

test('custom units never silently use standard conversion factors',()=>{assert.equal(convertMeasurement({width:5,width_unit:'Custom width',weight:20,weight_unit:'GSM'}).gramsPerMetre,null);assert.equal(convertMeasurement({width:5,width_unit:'cm',weight:20,weight_unit:'Custom weight'}).gsm,null)});
