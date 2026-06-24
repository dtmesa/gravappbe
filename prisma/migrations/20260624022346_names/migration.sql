/*
  Warnings:

  - A unique constraint covering the columns `[workoutId,name]` on the table `Exercise` will be added. If there are existing duplicate values, this will fail.
  - A unique constraint covering the columns `[userId,name]` on the table `Workout` will be added. If there are existing duplicate values, this will fail.

*/
-- CreateIndex
CREATE UNIQUE INDEX "Exercise_workoutId_name_key" ON "Exercise"("workoutId", "name");

-- CreateIndex
CREATE UNIQUE INDEX "Workout_userId_name_key" ON "Workout"("userId", "name");
