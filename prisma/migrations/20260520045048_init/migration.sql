/*
  Warnings:

  - You are about to drop the column `name` on the `ExerciseSession` table. All the data in the column will be lost.

*/
-- AlterTable
ALTER TABLE "ExerciseSession" DROP COLUMN "name";

-- AlterTable
ALTER TABLE "Workout" ADD COLUMN     "order" INTEGER NOT NULL DEFAULT 0;
