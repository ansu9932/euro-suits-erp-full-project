import {createTimeline} from 'animejs';
import type {BlazerGroups} from './blazerModel';

/*
 * The construction-story choreography for the blazer, kept free of WebGL so the
 * scroll-driven opening can be exercised in plain Node.
 */

export type LayerPose = {shell: number; inner: number; sleeve: number; trim: number; turn: number};

/** Opens the jacket apart for one point of the construction story. */
export function poseLayers(groups: BlazerGroups, pose: LayerPose) {
 const {shell, inner, sleeve, trim} = pose;
 groups.shellLeft.position.set(-shell * .52, 0, shell * .58);
 groups.shellRight.position.set(shell * .52, 0, shell * .58);
 groups.shellLeft.rotation.y = -shell * .28;
 groups.shellRight.rotation.y = shell * .28;
 groups.canvasLeft.position.set(-inner * .2, 0, inner * .1);
 groups.canvasRight.position.set(inner * .2, 0, inner * .1);
 groups.lining.position.set(0, -inner * .12, -inner * .38);
 groups.sleeveLeft.position.set(-sleeve * .66, sleeve * .06, sleeve * .1);
 groups.sleeveRight.position.set(sleeve * .66, sleeve * .06, sleeve * .1);
 groups.sleeveLeft.rotation.z = -sleeve * .07;
 groups.sleeveRight.rotation.z = sleeve * .07;
 groups.lapelLeft.position.set(-shell * .52 - trim * .1, trim * .1, shell * .58 + trim * .5);
 groups.lapelRight.position.set(shell * .52 + trim * .1, trim * .1, shell * .58 + trim * .5);
 groups.trimLeft.position.set(0, -trim * .12, trim * .62);
 groups.trimRight.position.set(0, -trim * .12, trim * .62);
}


/** One reversible timeline for the five construction chapters (0 to 4 seconds). */
export function createLayerTimeline(target: LayerPose) {
 return createTimeline({autoplay: false, defaults: {ease: 'inOut(3)'}})
  .add(target, {shell: [0, 1], duration: 850}, 150)
  .add(target, {inner: [0, 1], duration: 900}, 1100)
  .add(target, {sleeve: [0, 1], duration: 900}, 2100)
  .add(target, {trim: [0, 1], duration: 900}, 3100)
  .add(target, {turn: [-.2, -.5], duration: 4000, ease: 'linear'}, 0);
}

/** Camera distance that frames the whole garment for the current viewport. */
export function cameraDistance(height: number, fovDegrees: number, aspect: number, mode: 'hero' | 'layers', zoom = 1) {
 const half = Math.tan((fovDegrees * Math.PI) / 360);
 const needHeight = (height / 2) / half;
 const needWidth = (height * .42) / (half * Math.max(.4, aspect));
 return Math.max(needHeight, needWidth) * (mode === 'layers' ? 1.22 : 1.16) * zoom;
}
