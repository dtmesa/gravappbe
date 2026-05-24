/*
  Warnings:

  - You are about to drop the column `metricType` on the `Exercise` table. All the data in the column will be lost.

*/
-- CreateEnum
CREATE TYPE "MetricType" AS ENUM ('WEIGHT', 'DURATION', 'REPS', 'DISTANCE');

-- AlterTable
ALTER TABLE "Exercise" DROP COLUMN "metricType",
ADD COLUMN     "isDistance" BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN     "isDuration" BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN     "isReps" BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN     "isWeight" BOOLEAN NOT NULL DEFAULT false;

-- DropEnum
DROP TYPE "SetMetricType";
